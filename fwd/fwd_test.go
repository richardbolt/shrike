package fwd_test

import (
	"context"
	"hal/routes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"hal/fwd"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/testutils"
	"github.com/vulcand/oxy/utils"
)

func AddConfigContext(req *http.Request) *http.Request {
	return req.WithContext(
		routes.ContextWithConfig(req.Context(), routes.Config{
			DownstreamProxyURL: *req.URL,
		}),
	)
}

var _ = Describe("Fwd", func() {
	Describe("Headers", func() {
		It("Removes hop headers", func() {
			called := false
			var outHeaders http.Header
			var outHost, expectedHost string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				called = true
				outHeaders = req.Header
				outHost = req.Host
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				expectedHost = req.URL.Host
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			for _, h := range fwd.HopHeaders {
				headers := http.Header{
					fwd.Connection: []string{"close"},
					h:              []string{"Hop Header", "For", "sure"},
				}

				re, body, err := testutils.Get(proxy.URL, testutils.Headers(headers))
				Expect(err).To(BeNil())
				Expect(string(body)).To(Equal("hello"))
				Expect(re.StatusCode).To(Equal(http.StatusOK))
				Expect(called).To(BeTrue())
				Expect(outHeaders.Get(fwd.Connection)).To(Equal(""))
				Expect(outHeaders.Get(h)).To(Equal(""))
				Expect(outHost).To(Equal(expectedHost))
			}
		})

		It("Forwards non-hop headers", func() {
			var outHeaders http.Header
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				outHeaders = req.Header
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New(fwd.Rewriter(&fwd.HeaderRewriter{TrustForwardHeader: true, Hostname: "hello"}))
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			headers := http.Header{
				fwd.XForwardedProto:  []string{"httpx"},
				fwd.XForwardedFor:    []string{"192.168.1.1"},
				fwd.XForwardedServer: []string{"foobar"},
				fwd.XForwardedHost:   []string{"upstream-foobar"},
			}

			re, _, err := testutils.Get(proxy.URL, testutils.Headers(headers))
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(outHeaders.Get(fwd.XForwardedProto)).To(Equal("httpx"))
			Expect(strings.Contains(outHeaders.Get(fwd.XForwardedFor), "192.168.1.1")).To(BeTrue())
			Expect(strings.Contains(outHeaders.Get(fwd.XForwardedHost), "upstream-foobar")).To(BeTrue())
			Expect(outHeaders.Get(fwd.XForwardedServer)).To(Equal("hello"))
		})

		It("Header rewriter rewrites headers", func() {
			var outHeaders http.Header
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				outHeaders = req.Header
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New(fwd.Rewriter(&fwd.HeaderRewriter{TrustForwardHeader: false, Hostname: "hello"}))
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			headers := http.Header{
				fwd.XForwardedProto: []string{"httpx"},
				fwd.XForwardedFor:   []string{"192.168.1.1"},
			}

			re, _, err := testutils.Get(proxy.URL, testutils.Headers(headers))
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(outHeaders.Get(fwd.XForwardedProto)).To(Equal("http"))
			Expect(strings.Contains(outHeaders.Get(fwd.XForwardedFor), "192.168.1.1")).To(BeFalse())
		})

		It("Normalizes response headers that need to be singular", func() {
			called := false
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				called = true
				headers := w.Header()
				for _, k := range fwd.SingularHeaders {
					headers.Add(k, "fred")
					headers.Add(k, "john")
				}
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			re, _, err := testutils.Get(proxy.URL, testutils.Headers(http.Header{}))
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(called).To(BeTrue())
			for _, k := range fwd.SingularHeaders {
				Expect(re.Header[k]).To(Equal([]string{"fred"}))
			}
		})
	})

	Describe("Error handling", func() {
		It("Default error handler works", func() {
			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI("http://localhost:63450")
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			re, _, err := testutils.Get(proxy.URL)
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusBadGateway))
		})

		It("Custom error handler works", func() {
			f, err := fwd.New(fwd.ErrorHandler(utils.ErrorHandlerFunc(func(w http.ResponseWriter, req *http.Request, err error) {
				w.WriteHeader(http.StatusTeapot)
				w.Write([]byte(http.StatusText(http.StatusTeapot)))
			})))
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI("http://localhost:63450")
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			re, body, err := testutils.Get(proxy.URL)
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusTeapot))
			Expect(string(body)).To(Equal(http.StatusText(http.StatusTeapot)))
		})
	})

	Describe("Transport timeouts", func() {
		It("Custom transport timeout times out appropriately", func() {
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(20 * time.Millisecond)
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New(fwd.RoundTripper(
				&http.Transport{
					ResponseHeaderTimeout: 5 * time.Millisecond,
				}))
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			re, _, err := testutils.Get(proxy.URL)
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusGatewayTimeout))
		})
	})

	Describe("Escaped urls", func() {
		It("Escaped urls proxy correctly", func() {
			var outURL string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				outURL = req.RequestURI
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			path := "/log/http%3A%2F%2Fwww.site.com%2Fsomething?a=b"

			request, err := http.NewRequest("GET", proxy.URL, nil)
			parsed := testutils.ParseURI(proxy.URL)
			parsed.Opaque = path
			request.URL = parsed
			re, err := http.DefaultClient.Do(request)
			Expect(err).To(BeNil())
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(outURL).To(Equal(path))
		})
	})

	Describe("Proxies traffic correctly", func() {
		It("Proxies http traffic", func() {
			var proto string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				proto = req.Header.Get(fwd.XForwardedProto)
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			re, body, err := testutils.Get(proxy.URL)
			Expect(err).To(BeNil())
			Expect(proto).To(Equal("http"))
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(Equal("hello"))
		})

		It("Proxies https traffic", func() {
			var proto string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				proto = req.Header.Get(fwd.XForwardedProto)
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			tproxy := httptest.NewUnstartedServer(proxy)
			tproxy.StartTLS()
			defer tproxy.Close()

			re, body, err := testutils.Get(tproxy.URL)
			Expect(err).To(BeNil())
			Expect(proto).To(Equal("https"))
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(Equal("hello"))
		})

		It("Proxies to another Edgy ", func() {
			var proto string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				proto = req.Header.Get(fwd.XForwardedProto)
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			tproxy := httptest.NewUnstartedServer(proxy)
			tproxy.StartTLS()
			defer tproxy.Close()

			re, body, err := testutils.Get(tproxy.URL)
			Expect(err).To(BeNil())
			Expect(proto).To(Equal("https"))
			Expect(re.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(Equal("hello"))
		})

		It("Should respect the request context's done channel", func() {
			var proto string
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				proto = req.Header.Get(fwd.XForwardedProto)
				time.Sleep(200 * time.Millisecond)
				w.Write([]byte("hello"))
			})
			defer srv.Close()

			f, err := fwd.New()
			Expect(err).To(BeNil())

			proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				req.URL = testutils.ParseURI(srv.URL)
				f.ServeHTTP(w, AddConfigContext(req))
			})
			tproxy := httptest.NewUnstartedServer(proxy)
			tproxy.StartTLS()
			defer tproxy.Close()

			req, _ := http.NewRequest(http.MethodGet, tproxy.URL, nil)
			ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
			defer cancel()
			req = req.WithContext(ctx)

			client := &http.Client{}

			res, err := client.Do(req)

			Expect(err).Should(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("Proxies WebSocket traffic", func() {
			f, err := fwd.New()
			Expect(err).To(BeNil())

			mux := http.NewServeMux()
			mux.Handle("/ws", &wsEchoHandler{upgrader: websocket.Upgrader{}})
			srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				mux.ServeHTTP(w, req)
			})
			defer srv.Close()

			proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
				path := req.URL.Path // keep the original path
				// Set new backend URL
				req.URL = testutils.ParseURI(srv.URL)
				req.URL.Path = path
				f.ServeHTTP(w, AddConfigContext(req))
			})
			defer proxy.Close()

			proxyAddr := proxy.Listener.Addr().String()
			resp, err := sendWebSocketRequest(proxyAddr, "/ws", "echo")
			Expect(err).To(BeNil())
			Expect(resp).To(Equal("echo"))
		})

		/*
			// TODO: Why aren't we handling chunked responses correctly? vulcand/oxy/forward is also failing at this test.
			It("Handles chunked upstream responses", func() {
				srv := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
					h := w.(http.Hijacker)
					conn, _, _ := h.Hijack()
					fmt.Fprintf(conn, "HTTP/1.0 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\ntest\r\n5\r\ntest1\r\n5\r\ntest2\r\n0\r\n\r\n")
					conn.Close()
				})
				defer srv.Close()

				f, err := fwd.New()
				Expect(err).To(BeNil())

				proxy := testutils.NewHandler(func(w http.ResponseWriter, req *http.Request) {
					req.URL = testutils.ParseURI(srv.URL)
					f.ServeHTTP(w, AddConfigContext(req))
				})
				defer proxy.Close()

				re, body, err := testutils.Get(proxy.URL)
				Expect(err).To(BeNil())
				Expect(re.StatusCode).To(Equal(http.StatusOK))
				Expect(string(body)).To(Equal("testtest1test2"))
				Expect(re.Header.Get("Content-Length")).To(HaveLen(len("testtest1test2")))
			})
		*/
	})
})

// sendWebSocketRequest is a test helper to send a WebSocket request.
func sendWebSocketRequest(serverAddr, path, data string) (received string, err error) {
	u := url.URL{Scheme: "ws", Host: serverAddr, Path: path}
	log.Printf("connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	err = conn.WriteMessage(websocket.TextMessage, []byte(data))
	if err != nil {
		log.Debug("write:", err)
		return
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Debug("read:", err)
		return
	}
	log.Debug("recv: %s", string(message))
	return string(message), nil
}

// wsEchoHandler is an http.Handler that echoes WebSocket frames back to the client.
type wsEchoHandler struct {
	upgrader websocket.Upgrader
}

func (s *wsEchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Debug("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Debug("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Debug("write:", err)
			break
		}
	}
}
