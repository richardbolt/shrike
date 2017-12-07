package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"

	"shrike/api"
	"shrike/cfg"
	"shrike/mux"

	toxy "github.com/Shopify/toxiproxy/client"
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

	go mux.ServeMux(func(c *mux.Config) {
		c.DownstreamProxyURL = *d
		c.ToxyAddress = cfg.ToxyAddress
		c.ToxyAPIPort = cfg.ToxyAPIPort
		c.ToxyNamePathSeparator = cfg.ToxyNamePathSeparator
		c.ToxyClient = toxy.NewClient(c.ToxyAPIAddress())
	})

	log.WithFields(log.Fields{
		"host": cfg.Host,
		"port": cfg.Port,
	}).Info("Proxy HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), nil)
	}()

	c := mux.Config{
		DownstreamProxyURL:    *d,
		ToxyAddress:           cfg.ToxyAddress,
		ToxyAPIPort:           cfg.ToxyAPIPort,
		ToxyNamePathSeparator: cfg.ToxyNamePathSeparator,
	}
	c.ToxyClient = toxy.NewClient(c.ToxyAPIAddress())

	apiMux := api.New(c)
	log.WithFields(log.Fields{
		"host": cfg.Host,
		"port": cfg.APIPort,
	}).Info("API HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf(":%d", cfg.APIPort), apiMux)
	}()

	log.Fatal(<-errc)
}
