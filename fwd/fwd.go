// Package fwd implements http handler that forwards requests to remote server
// and serves back the response. Based strongly on https://github.com/vulcand/oxy/forward. Apache Licensed.
// WebSocket forwarder is based on https://github.com/koding/websocketproxy. MIT licensed.
package fwd

import (
	"hal/routes"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/utils"
)

var (
	// DefaultWebsocketUpgrader specifies the parameters for upgrading an HTTP
	// connection to a WebSocket connection.
	DefaultWebsocketUpgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Disable origin checking - this is a forwarding proxy.
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// DefaultWebsocketDialer is a dialer with all fields set to the default zero values.
	DefaultWebsocketDialer = websocket.DefaultDialer
)

// ReqRewriter can alter request headers and body
type ReqRewriter interface {
	Rewrite(r *http.Request)
}

type OptSetter func(f *Forwarder) error

// PassHostHeader specifies if a client's Host header field should
// be delegated
func PassHostHeader(b bool) OptSetter {
	return func(f *Forwarder) error {
		f.httpForwarder.passHost = b
		return nil
	}
}

// RoundTripper sets a new http.RoundTripper
// Forwarder will use http.DefaultTransport as a default round tripper
func RoundTripper(r http.RoundTripper) OptSetter {
	return func(f *Forwarder) error {
		f.roundTripper = r
		return nil
	}
}

// Dialer mirrors the net.Dial function to be able to define alternate
// implementations
type Dialer func(network, address string) (net.Conn, error)

// Rewriter defines a request rewriter for the HTTP forwarder
func Rewriter(r ReqRewriter) OptSetter {
	return func(f *Forwarder) error {
		f.httpForwarder.rewriter = r
		return nil
	}
}

// ErrorHandler is a functional argument that sets error handler of the server
func ErrorHandler(h utils.ErrorHandler) OptSetter {
	return func(f *Forwarder) error {
		f.errHandler = h
		return nil
	}
}

func Stream(stream bool) OptSetter {
	return func(f *Forwarder) error {
		f.stream = stream
		return nil
	}
}

func StateListener(stateListener UrlForwardingStateListener) OptSetter {
	return func(f *Forwarder) error {
		f.stateListener = stateListener
		return nil
	}
}

func StreamingFlushInterval(flushInterval time.Duration) OptSetter {
	return func(f *Forwarder) error {
		f.httpStreamingForwarder.flushInterval = flushInterval
		return nil
	}
}

// Forwarder wraps two traffic forwarding implementations: HTTP and websockets.
// It decides based on the specified request which implementation to use
type Forwarder struct {
	*httpForwarder
	*httpStreamingForwarder
	*websocketForwarder
	*handlerContext
	stateListener UrlForwardingStateListener
	stream        bool
}

// handlerContext defines a handler context for error reporting and logging
type handlerContext struct {
	errHandler utils.ErrorHandler
}

// httpForwarder is a handler that can reverse proxy
// HTTP traffic
type httpForwarder struct {
	roundTripper http.RoundTripper
	rewriter     ReqRewriter
	passHost     bool
}

// httpStreamingForwarder is a handler that can reverse proxy
// HTTP traffic but doesn't wait for a complete
// response before it begins writing bytes upstream
type httpStreamingForwarder struct {
	rewriter      ReqRewriter
	passHost      bool
	flushInterval time.Duration
}

// websocketForwarder is a handler that can reverse proxy
// websocket traffic
type websocketForwarder struct {
	// Upgrader specifies the parameters for upgrading a incoming HTTP
	// connection to a WebSocket connection. If nil, DefaultUpgrader is used.
	Upgrader *websocket.Upgrader

	//  Dialer contains options for connecting to the backend WebSocket server.
	//  If nil, DefaultDialer is used.
	Dialer *websocket.Dialer
}

const (
	StateConnected = iota
	StateDisconnected
)

type UrlForwardingStateListener func(*url.URL, int)

// New creates an instance of Forwarder based on the provided list of configuration options
func New(setters ...OptSetter) (*Forwarder, error) {
	h, err := os.Hostname()
	if err != nil {
		h = "localhost"
	}

	f := &Forwarder{
		httpForwarder: &httpForwarder{
			roundTripper: http.DefaultTransport,
			rewriter: &HeaderRewriter{
				TrustForwardHeader: true,
				Hostname:           h,
			},
		},
		httpStreamingForwarder: &httpStreamingForwarder{
			flushInterval: time.Duration(100) * time.Millisecond,
		},
		websocketForwarder: &websocketForwarder{
			Dialer:   DefaultWebsocketDialer,
			Upgrader: DefaultWebsocketUpgrader,
		},
		handlerContext: &handlerContext{
			errHandler: utils.DefaultHandler,
		},
	}

	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

// ServeHTTP decides which forwarder to use based on the specified
// request and delegates to the proper implementation
func (f *Forwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f.stateListener != nil {
		f.stateListener(req.URL, StateConnected)
		defer f.stateListener(req.URL, StateDisconnected)
	}

	var target *url.URL

	// TODO: Here is where we set the url and port to go to.
	// Either the Toxy or the downstream address.
	target = routes.GetHostFor(req)

	if target == nil {
		// No target found for the given request.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"message":"Could not find where to proxy your request."}`))
		log.Error("No target found to proxy to.")
		return
	}

	req.URL.Host = target.Host
	req.Host = target.Host // Sets the Host header which is important for some sites.

	if websocket.IsWebSocketUpgrade(req) {
		if target.Scheme == "https" {
			target.Scheme = "wss"
		} else {
			target.Scheme = "ws"
		}
		f.websocketForwarder.serveHTTP(w, req, f.handlerContext, target)
	} else if f.stream {
		req.URL.Scheme = target.Scheme
		f.httpStreamingForwarder.serveHTTP(w, req, f.handlerContext)
	} else {
		req.URL.Scheme = target.Scheme
		f.httpForwarder.serveHTTP(w, req, f.handlerContext)
	}
}

// serveHTTP forwards HTTP traffic using the configured transport
func (f *httpForwarder) serveHTTP(w http.ResponseWriter, req *http.Request, ctx *handlerContext) {
	start := time.Now().UTC()
	rc := make(chan *http.Response)
	errc := make(chan error)

	reqCopy := f.copyRequest(req, req.URL)
	reqCtx := reqCopy.Context()

	// Spin off our roundtrip in a go routine so we can collect the response or error while
	// paying attention to the request context's done channel.
	go func() {
		response, err := f.roundTripper.RoundTrip(reqCopy)

		if err != nil {
			errc <- err
			return
		}

		rc <- response
	}()

	select {
	// Happy path, we get a response from our proxied request.
	case response := <-rc:
		fields := log.Fields{
			"url":      req.URL,
			"status":   response.StatusCode,
			"duration": time.Now().UTC().Sub(start),
		}
		msg := "Round trip."
		if req.TLS != nil {
			msg = "TLS round trip."
		}
		if response.StatusCode < http.StatusMultipleChoices {
			log.WithFields(fields).Debug(msg)
		} else {
			log.WithFields(fields).Info(msg)
		}

		CopyHeaders(w.Header(), response.Header)
		w.WriteHeader(response.StatusCode)

		written, err := io.Copy(w, response.Body)
		defer response.Body.Close()

		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error copying upstream response Body.")
			ctx.errHandler.ServeHTTP(w, req, err)
			return
		}

		if written != 0 {
			w.Header().Set(ContentLength, strconv.FormatInt(written, 10))
		}
	// There was an error with the proxied request
	case err := <-errc:
		log.WithFields(log.Fields{
			"url":   req.URL,
			"error": err,
		}).Error("Error forwarding.")
		ctx.errHandler.ServeHTTP(w, req, err)
	// The request was canceled, so end early.
	case <-reqCtx.Done():
		log.WithFields(log.Fields{
			"url": req.URL,
			"err": reqCtx.Err(),
		}).Error("Context Done")
		ctx.errHandler.ServeHTTP(w, req, reqCtx.Err())
	}
}

// copyRequest makes a copy of the specified request to be sent using the configured
// transport
func (f *httpForwarder) copyRequest(req *http.Request, u *url.URL) *http.Request {
	outReq := new(http.Request)
	*outReq = *req // includes shallow copies of maps, but we handle this below

	outReq.URL = utils.CopyURL(req.URL)
	outReq.URL.Scheme = u.Scheme
	outReq.URL.Host = u.Host
	outReq.URL.Opaque = req.RequestURI
	// raw query is already included in RequestURI, so ignore it to avoid dupes
	outReq.URL.RawQuery = ""
	// Do not pass client Host header unless optsetter PassHostHeader is set.
	if !f.passHost {
		outReq.Host = u.Host
	}
	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1

	// Overwrite close flag so we can keep persistent connection for the backend servers
	outReq.Close = false

	outReq.Header = make(http.Header)
	CopyHeaders(outReq.Header, req.Header)

	if f.rewriter != nil {
		f.rewriter.Rewrite(outReq)
	}
	return outReq
}

/*
The MIT License (MIT)

Copyright (c) 2014 Koding, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
// serveHTTP forwards websocket traffic based on this (MIT Licensed) code: https://github.com/koding/websocketproxy
func (w *websocketForwarder) serveHTTP(rw http.ResponseWriter, req *http.Request, ctx *handlerContext, target *url.URL) {
	// Pass headers from the incoming request to the dialer to forward them to
	// the final destinations.
	requestHeader := http.Header{}
	if origin := req.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin", origin)
	}
	for _, prot := range req.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range req.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}

	// Set the target parameters.
	target.Fragment = req.URL.Fragment
	target.Path = req.URL.Path
	target.RawQuery = req.URL.RawQuery

	// Pass X-Forwarded-For headers too, code below is a part of
	// httputil.ReverseProxy. See http://en.wikipedia.org/wiki/X-Forwarded-For
	// for more information
	// TODO: use RFC7239 http://tools.ietf.org/html/rfc7239
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	// Set the originating protocol of the incoming HTTP request. The SSL might
	// be terminated on our site and because we doing proxy adding this would
	// be helpful for applications on the backend.
	requestHeader.Set("X-Forwarded-Proto", "http")
	if req.TLS != nil {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	// Connect to the backend URL, also pass the headers we get from the request
	// together with the Forwarded headers we prepared above.
	// TODO: support multiplexing on the same backend connection instead of
	// opening a new TCP connection time for each request. This should be
	// optional:
	// http://tools.ietf.org/html/draft-ietf-hybi-websocket-multiplexing-01
	connBackend, resp, err := w.Dialer.Dial(target.String(), requestHeader)
	if err != nil {
		log.WithFields(log.Fields{
			"module": "fwd.websocket",
			"error":  err,
			"url":    target.String(),
			"scheme": target.Scheme,
			"status": resp.StatusCode,
		}).Error("Couldn't dial to remote backend url.")
		ctx.errHandler.ServeHTTP(rw, req, err)
		return
	}
	log.WithFields(log.Fields{
		"module": "fwd.websocket",
		"url":    req.URL.Path,
	}).Debug("Dialed to remote backend url.")
	defer connBackend.Close()

	upgrader := w.Upgrader
	if w.Upgrader == nil {
		upgrader = DefaultWebsocketUpgrader
	}

	// Only pass those headers to the upgrader.
	upgradeHeader := http.Header{}
	if hdr := resp.Header.Get("Sec-Websocket-Protocol"); hdr != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", hdr)
	}
	if hdr := resp.Header.Get("Set-Cookie"); hdr != "" {
		upgradeHeader.Set("Set-Cookie", hdr)
	}

	// Now upgrade the existing incoming request to a WebSocket connection.
	// Also pass the header that we gathered from the Dial handshake.
	connPub, err := upgrader.Upgrade(rw, req, upgradeHeader)
	if err != nil {
		log.WithFields(log.Fields{
			"module": "fwd.websocket",
			"error":  err,
		}).Error("Couldn't upgrade.")
		return
	}
	defer connPub.Close()

	errc := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errc <- err
	}

	// Start our proxy now, everything is ready...
	go cp(connBackend.UnderlyingConn(), connPub.UnderlyingConn())
	go cp(connPub.UnderlyingConn(), connBackend.UnderlyingConn())
	<-errc
}

// serveHTTP forwards HTTP traffic using the configured transport
func (f *httpStreamingForwarder) serveHTTP(w http.ResponseWriter, req *http.Request, ctx *handlerContext) {
	pw := utils.ProxyWriter{
		W: w,
	}
	start := time.Now().UTC()

	reqURL, err := url.ParseRequestURI(req.RequestURI)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"URI":   req.RequestURI,
		}).Error("Error parsing Request URI.")
		ctx.errHandler.ServeHTTP(w, req, err)
		return
	}

	urlcpy := utils.CopyURL(req.URL)
	urlcpy.Scheme = req.URL.Scheme
	urlcpy.Host = req.URL.Host

	req.URL.Path = reqURL.Path
	req.URL.RawQuery = reqURL.RawQuery

	revproxy := httputil.NewSingleHostReverseProxy(urlcpy)
	revproxy.FlushInterval = f.flushInterval //Flush something every flushInterval (100 milliseconds is the default)
	revproxy.ServeHTTP(w, req)

	if req.TLS != nil {
		log.WithFields(log.Fields{
			"url":         req.URL,
			"code":        pw.Code,
			"length":      pw.Length,
			"duration":    time.Now().UTC().Sub(start),
			"tls:version": req.TLS.Version,
			"tls:resume":  req.TLS.DidResume,
			"tls:csuite":  req.TLS.CipherSuite,
			"tls:server":  req.TLS.ServerName,
		}).Debug("TLS round trip.")
	} else {
		log.WithFields(log.Fields{
			"url":      req.URL,
			"code":     pw.Code,
			"length":   pw.Length,
			"duration": time.Now().UTC().Sub(start),
		}).Debug("Round trip.")
	}
}
