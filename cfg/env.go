package cfg

import "github.com/kelseyhightower/envconfig"

// Env represents the possible environment variable config params.
type Env struct {
	Host        string `default:"0.0.0.0"`
	Port        int    `default:"8080"`
	APIPort     int    `envconfig:"API_PORT" default:"8475"`
	UpstreamURL string `envconfig:"UPSTREAM_URL" default:"http://localhost"`
}

// New returns a new configuration, populated from environment variables and/or defaults.
// NB: It will panic if defaults aren't set.
func New() Env {
	cfg := &Env{}
	envconfig.MustProcess("", cfg)
	return *cfg
}
