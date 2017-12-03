package routes

import "context"

// Key to use when setting the Config.
type ctxKeyConfig int

// ConfigKey is the key that holds the Config in a request context.
const ConfigKey ctxKeyConfig = 0

// ContextWithConfig will put a given pod into the passed context under a package private key/key type.
func ContextWithConfig(ctx context.Context, c Config) context.Context {
	return context.WithValue(ctx, ConfigKey, c)
}

// GetConfig pulls the config from the context.
func GetConfig(ctx context.Context) Config {
	return ctx.Value(ConfigKey).(Config)
}
