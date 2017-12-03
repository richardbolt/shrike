package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"

	"hal/cfg"
	"hal/mux"

	"github.com/pressly/lg"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfg := cfg.New()

	errc := make(chan error)

	// Logger setup, including redirect of stdout to logger.
	logger := log.New()
	lg.RedirectStdlogOutput(logger)
	lg.DefaultLogger = logger

	d, err := url.Parse(cfg.DownstreamProxyURL)
	if err != nil {
		logger.Fatalf("DOWNSTREAM_PROXY_URL must be a valid URI: %s", err)
	}

	mux.ServeMux(func(c *mux.Config) {
		c.DownstreamProxyURL = *d
		c.ToxyAddress = cfg.ToxyAddress
	})

	log.WithFields(log.Fields{
		"host": cfg.Host,
		"port": cfg.Port,
	}).Info("Proxy HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), nil)
	}()

	log.WithFields(log.Fields{
		"host": cfg.Host,
		"port": cfg.APIPort,
	}).Info("API HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf(":%d", cfg.APIPort), nil)
	}()

	log.Fatal(<-errc)
}
