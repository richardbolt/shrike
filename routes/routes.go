package routes

import (
	"net/http"
	"net/url"
)

// Config information.
type Config struct {
	// DownstreamProxyURL is where we proxy requests and where we route requests via the Toxy.
	DownstreamProxyURL url.URL
	// ToxyAddress is the ip or DNS address of your ToxyProxy instance.
	ToxyAddress string
}

// GetHostFor gets the host:port to forward the request to.
// Puss config from the context.
func GetHostFor(req *http.Request) *url.URL {
	c := GetConfig(req.Context())
	// TODO: Test req.URL.Path against a set of rules to see where we might actually sent it.
	return &c.DownstreamProxyURL
}

// NewConfigMW returns a middleware with a Config on the context.
func NewConfigMW(toxy string, downstream url.URL) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(res, req.WithContext(
				ContextWithConfig(req.Context(), Config{
					DownstreamProxyURL: downstream,
					ToxyAddress:        toxy,
				}),
			))
		})
	}
}
