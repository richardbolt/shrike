package cfg

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Env represents the possible environment variable config params.
type Env struct {
	Host                  string `default:"localhost"`
	Port                  int    `default:"8080"`
	APIPort               int    `envconfig:"API_PORT" default:"8475"`
	ToxyAddress           string `envconfig:"TOXY_ADDRESS" default:"127.0.0.1"`
	ToxyAPIPort           int    `envconfig:"TOXY_API_PORT" default:"8474"`
	ToxyNamePathSeparator string `envconfig:"TOXY_SEPARATOR" default:"__"`
	DownstreamProxyURL    string `envconfig:"DOWNSTREAM_PROXY_URL" default:"http://localhost"`
}

func (e *Env) ToxyAPIAddress() string {
	return fmt.Sprintf("%s:%d", e.ToxyAddress, e.ToxyAPIPort)
}

// New returns a new configuration, populated from environment variables and/or defaults.
// NB: It will panic if defaults aren't set.
func New() Env {
	cfg := &Env{}
	envconfig.MustProcess("", cfg)
	return *cfg
}
