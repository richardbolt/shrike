package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/richardbolt/shrike/mux"
	"github.com/richardbolt/shrike/routes"

	toxy "github.com/Shopify/toxiproxy/client"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	log "github.com/sirupsen/logrus"
)

// New API mux
func New(cfg mux.Config) *http.ServeMux {
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

	r.Get("/routes", GetProxiesWith(cfg))

	r.Post("/routes", AddProxyWith(cfg))

	r.Delete("/routes/{route}", DeleteRouteWith(cfg))

	mux.Handle("/", r)
	return mux
}

// Route holds information about the routing of this request
type Route struct {
	Path string `json:"path"`
}

// RouteWithProxy has the proxy and path information to show clients
type RouteWithProxy struct {
	Path string      `json:"path"`
	Toxy *toxy.Proxy `json:"toxy"`
}

// GetProxiesWith gets proxies from Toxiproxy. Basically just wraps the ToxiProxy data for now.
// Intended to show the actual path that will match as well
func GetProxiesWith(cfg mux.Config) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		proxies, err := cfg.ToxyClient.Proxies()
		if err != nil {
			log.WithField("err", err).Error("Error getting proxies")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status": "Server Error", "message": "Could not create the path entry."}`))
			return
		}

		routeMap := map[string]RouteWithProxy{}
		for k, v := range proxies {
			path := routes.PathNameFrom(cfg.ToxyNamePathSeparator, k)
			routeMap[k] = RouteWithProxy{
				Path: path,
				Toxy: v,
			}
		}

		b, _ := json.Marshal(routeMap)
		w.Write(b)
	}
}

// AddProxyWith cfg to Toxiproxy.
func AddProxyWith(cfg mux.Config) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Decode payload into a Route
		defer req.Body.Close()
		body, _ := ioutil.ReadAll(req.Body)
		doc := &Route{}
		if err := json.Unmarshal(body, &doc); err != nil {
			log.Errorf("Error unmarshaling body %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status": "Bad Request", "message": "request body is not a valid JSON object."}`))
			return
		}
		proxyName := routes.ProxyNameFrom(cfg.ToxyNamePathSeparator, doc.Path)
		proxy, err := cfg.ToxyClient.CreateProxy(
			proxyName,
			fmt.Sprintf("%s:%d", cfg.ToxyAddress, routes.NumFrom(proxyName)),
			cfg.DownstreamProxyURL.Host,
		)
		if err != nil {
			proxy, err = cfg.ToxyClient.Proxy(proxyName)
			if err != nil {
				log.WithField("err", err).Error("Error creating/getting a proxy")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"status": "Server Error", "message": "Could not create the path entry."}`))
				return
			}
		}

		b, _ := json.Marshal(proxy)
		w.Write(b)
	}
}

// DeleteRouteWith cfg removes a route from the proxy
func DeleteRouteWith(cfg mux.Config) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		route := chi.URLParam(req, "route")
		if route == "" {
			log.WithField("Route", route).Info("Route must be the name of one of the proxy paths.")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status": "Bad Request", "message": "Route must be the name of one of the proxy paths."}`))
			return
		}

		proxy, err := cfg.ToxyClient.Proxy(route)
		if err != nil {
			log.WithFields(log.Fields{
				"Route": route,
				"err":   err,
			}).Error("Error getting proxy to delete")
			w.WriteHeader(http.StatusGone)
			w.Write([]byte(`{"status": "No Proxy", "message": "No proxy by that name to remove."}`))
			return
		}
		err = proxy.Delete()
		if err != nil {
			log.WithFields(log.Fields{
				"Route": route,
				"err":   err,
			}).Error("Error deleting a proxy")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status": "Server Error", "message": "Could not delete the proxy."}`))
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
