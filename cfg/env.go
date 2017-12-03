package cfg

import "github.com/kelseyhightower/envconfig"

// Env represents the possible environment variable config params.
type Env struct {
	Host               string `default:"localhost"`
	Port               int    `default:"8080"`
	APIPort            int    `envconfig:"API_PORT" default:"8475"`
	ToxyAddress        string `envconfig:"TOXY_ADDRESS" default:"127.0.0.1"`
	DownstreamProxyURL string `envconfig:"DOWNSTREAM_PROXY_URL" default:"http://localhost"`
}

// New returns a new configuration, populated from environment variables and/or defaults.
// NB: It will panic if defaults aren't set.
func New() Env {
	cfg := &Env{}
	envconfig.MustProcess("", cfg)
	return *cfg
}
