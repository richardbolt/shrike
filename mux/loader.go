package mux

import (
	"net/http"

	"hal/routes"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	log "github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/forward"
)

type (
	// Config holds information about config loading.
	Config struct {
		// DownstreamProxyURL is where we proxy requests and where we route requests via the Toxy.
		DownstreamProxyURL string
		// ToxyAddress is the ip or DNS address of your ToxyProxy instance.
		ToxyAddress string
	}
	// Option allows for configuration.
	Option func(c *Config)
)

// ServeMux sets a new mux to http.DefaultServeMux
func ServeMux(options ...Option) {
	c := &Config{}

	for _, o := range options {
		o(c)
	}

	http.DefaultServeMux = New(c)
}

// New returns a new ServeMux to replace swap out when a new configuration is loaded.
func New(c *Config) *http.ServeMux {
	// Forwards incoming requests to whatever location URL points to, adds proper forwarding headers
	fwd, err := forward.New()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to create a new proxy forwarder.")
	}

	// Logger setup, including redirect of stdout to logger.
	logger := log.New()

	mux := http.NewServeMux()
	r := chi.NewRouter()
	// Chain HTTP Middleware
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(middleware.RequestID)
	log.Debug("RequestID middleware loaded into mux.")
	r.Use(middleware.Recoverer)
	log.Debug("Recoverer middleware loaded into mux.")
	r.Use(lg.RequestLogger(logger))
	log.Debug("RequestLogger middleware loaded into mux.")

	/*
		// Set a timeout value on the request context (ctx), that will signal
		// through ctx.Done() that the request has timed out and further
		// processing should be stopped.
		r.Use(middleware.Timeout(60 * time.Second))
		log.Debug("Timeout middleware loaded into mux with a 60 second timeout.")
	*/

	r.Use(routes.NewConfigMW(c.ToxyAddress, c.DownstreamProxyURL))
	log.Debug("NewConfigMW middleware loaded into mux.")

	proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// TODO: Here is where we set the url and port to go to.
		// Either the Toxy or the downstream address.
		req.URL = routes.GetHostFor(req)
		fwd.ServeHTTP(w, req)
	})

	// The primary handler.
	r.Handle("/*", proxy)

	mux.Handle("/", r) // Assign the new router to work with the primary multiplexing provided with chi.Mux.

	return mux
}
