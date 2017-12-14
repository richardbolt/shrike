package main

import (
	"flag"
	_ "net/http/pprof"

	"github.com/pressly/lg"
	"github.com/richardbolt/shrike/api"
	"github.com/richardbolt/shrike/cfg"
	log "github.com/sirupsen/logrus"
)

var host string
var port int
var apiPort int
var upstreamURL string

func main() {
	// Redirect stdout to logrus.
	logger := log.New()
	lg.RedirectStdlogOutput(logger)
	lg.DefaultLogger = logger

	cfg := cfg.New()
	flag.StringVar(&host, "host", cfg.Host, "Host for The Shrike to listen on")
	flag.IntVar(&port, "port", cfg.Port, "Port for The Shrike to listen on")
	flag.IntVar(&apiPort, "apiport", cfg.APIPort, "Port for The Shrike's API to listen on")
	flag.StringVar(&upstreamURL, "upstream", cfg.UpstreamURL, "Upstream URL to forward traffic to")
	flag.Parse()

	server := api.New(api.Config{
		Host:                  host,
		Port:                  port,
		APIPort:               apiPort,
		ToxyAddress:           "127.0.0.1",
		ToxyAPIPort:           8474,
		ToxyNamePathSeparator: "__",
		UpstreamURL:           upstreamURL,
	})

	server.Listen()
}
