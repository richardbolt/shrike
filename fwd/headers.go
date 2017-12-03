package fwd

import "net/http"

const (
	XForwardedProto    = "X-Forwarded-Proto"
	XForwardedFor      = "X-Forwarded-For"
	XForwardedHost     = "X-Forwarded-Host"
	XForwardedServer   = "X-Forwarded-Server"
	Connection         = "Connection"
	KeepAlive          = "Keep-Alive"
	ProxyAuthenticate  = "Proxy-Authenticate"
	ProxyAuthorization = "Proxy-Authorization"
	Te                 = "Te" // canonicalized version of "TE"
	Trailers           = "Trailers"
	TransferEncoding   = "Transfer-Encoding"
	Upgrade            = "Upgrade"
	ContentLength      = "Content-Length"

	AccessControlAllowOrigin = "Access-Control-Allow-Origin"
)

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
// Copied from reverseproxy.go, too bad
var HopHeaders = []string{
	Connection,
	KeepAlive,
	ProxyAuthenticate,
	ProxyAuthorization,
	Te, // canonicalized version of "TE"
	Trailers,
	TransferEncoding,
	Upgrade,
}

// SingularHeaders is a list of headers that should only appear once. Because we're a proxy and we have
// middleware inserting headers and then we naively copy headers from the round trip response into full response
// thus sometimes we can have undesireable impact on response HTTP headers with duplicates.
// This is most common with non-OPTIONS CORS requests when underlying services also try to handle them.
// NB: `Vary: origin` also seems to get set twice on /playbooks/v2/plays and /crm/v1/teams.
var SingularHeaders = []string{
	AccessControlAllowOrigin,
}

// CopyHeaders copies http headers from source to destination, it does not overide
// except for the SingularHeaders which can only be set once.
func CopyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
	// Make sure we have only one of the SingularHeaders set.
	for _, k := range SingularHeaders {
		v := dst.Get(k)
		if v != "" {
			dst.Set(k, v)
		}
	}
}
