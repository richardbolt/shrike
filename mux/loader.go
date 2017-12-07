package mux

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"shrike/routes"

	toxy "github.com/Shopify/toxiproxy/client"
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
		DownstreamProxyURL url.URL
		// ToxyAddress is the ip or DNS address of your ToxyProxy instance.
		ToxyAddress string
		// ToxyAPIPort is the port that api calls to Toxiproxy are made to.
		ToxyAPIPort int
		// ToxyNamePathSeparator is the string used to sub with / for storing path info sans slashes in Toxiproxy
		ToxyNamePathSeparator string
		// Toxiproxy client
		ToxyClient *toxy.Client
	}
	// Option allows for configuration.
	Option func(c *Config)
)

func (c *Config) ToxyAPIAddress() string {
	return fmt.Sprintf("%s:%d", c.ToxyAddress, c.ToxyAPIPort)
}

// ServeMux sets a new mux to http.DefaultServeMux
// Loops over
func ServeMux(options ...Option) {
	c := &Config{}

	for _, o := range options {
		o(c)
	}

	var prevHash *string
	for {
		proxies, err := c.ToxyClient.Proxies()
		if err != nil {
			log.Error(err)
			time.Sleep(1 * time.Second)
		}
		names := []string{}
		for proxyName := range proxies {
			names = append(names, proxyName)
		}
		sort.Strings(names)
		var hash *string
		hash = ptr(strings.Join(names, ""))
		if prevHash == nil || hash != prevHash {
			prevHash = hash
			http.DefaultServeMux = New(c)
		}
		time.Sleep(10)
	}
}

// ptr to a string
func ptr(s string) *string {
	return &s
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
	store := routes.NewProxyStore(c.DownstreamProxyURL, c.ToxyNamePathSeparator, c.ToxyClient)
	_ = store.Populate()

	// Logger setup, including redirect of stdout to logger.
	logger := log.New()

	mux := http.NewServeMux()
	r := chi.NewRouter()
	// Chain HTTP Middleware
	r.Use(middleware.Heartbeat("/ping"))
	log.Debug("/ping middleware loaded into mux.")

	r.Use(middleware.RequestID)
	log.Debug("RequestID middleware loaded into mux.")

	r.Use(middleware.Recoverer)
	log.Debug("Recoverer middleware loaded into mux.")

	r.Use(lg.RequestLogger(logger))
	log.Debug("RequestLogger middleware loaded into mux.")

	proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Either a proxy on the Toxy or the vanilla downstream address.
		if u, m := store.Match(req.URL.Path); m {
			req.URL = &u
		} else {
			req.URL = &c.DownstreamProxyURL
		}
		fwd.ServeHTTP(w, req)
	})

	// The primary handler.
	r.Handle("/*", proxy)

	mux.Handle("/", r) // Assign the new router to work with the primary multiplexing provided with chi.Mux.

	return mux
}
