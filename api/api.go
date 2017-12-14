package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Shopify/toxiproxy"
	toxy "github.com/Shopify/toxiproxy/client"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	"github.com/richardbolt/shrike/routes"
	log "github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/forward"
)

// New Shrike Server.
func New(c Config) *ShrikeServer {
	fwd, err := forward.New()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to create a new proxy forwarder.")
	}

	d, err := url.Parse(c.UpstreamURL)
	if err != nil {
		log.Fatalf("PROXY_URL must be a valid URI: %s", err)
	}

	return &ShrikeServer{
		cfg:        c,
		client:     toxy.NewClient(fmt.Sprintf("%s:%d", c.ToxyAddress, c.ToxyAPIPort)),
		fwd:        fwd,
		upstream:   d,
		toxiproxy:  toxiproxy.NewServer(),
		ProxyStore: routes.NewProxyStore(*d, c.ToxyNamePathSeparator),
	}
}

// Config for ShrikeServer
type Config struct {
	Host                  string
	Port                  int
	APIPort               int
	ToxyAddress           string
	ToxyAPIPort           int
	ToxyNamePathSeparator string
	UpstreamURL           string
}

// Route holds information about the routing of a request
type Route struct {
	Prefix string `json:"prefix"`
}

// RouteWithProxy has the proxy and path information to show clients
type RouteWithProxy struct {
	Route Route       `json:"route"`
	Toxy  *toxy.Proxy `json:"toxy"`
}

// JSONError is a simple error data structure
type JSONError struct {
	Status  string `json:"string"`
	Message string `json:"message"`
}

// Bytes for passing to http.ResponseWriter.Write()
func (j *JSONError) Bytes() []byte {
	b, _ := json.Marshal(j)
	return b
}

// ShrikeServer is the main Shrike server.
// Usage:
// server := api.New(..args)
// server.Listen()
type ShrikeServer struct {
	cfg        Config
	client     *toxy.Client
	upstream   *url.URL
	toxiproxy  *toxiproxy.ApiServer
	fwd        *forward.Forwarder
	ProxyStore *routes.ProxyStore
}

// Listen on all the appropriate ports
func (s *ShrikeServer) Listen() {
	errc := make(chan error)
	logger := log.New()

	// Toxiproxy API Server on ToxyAPIPort (8474)
	go func() {
		s.toxiproxy.Listen(s.cfg.ToxyAddress, strconv.Itoa(s.cfg.ToxyAPIPort))
	}()

	// Shrike API Server on APIPort (8475)
	apiMux := http.NewServeMux()
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(lg.RequestLogger(logger))

	r.Get("/routes", s.GetProxies)
	r.Post("/routes", s.AddProxy)
	r.Get("/routes/{route}", s.GetRoute)
	r.Delete("/routes/{route}", s.DeleteRoute)

	apiMux.Handle("/", r)

	log.WithFields(log.Fields{
		"host": s.cfg.Host,
		"port": s.cfg.APIPort,
	}).Info("API HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.APIPort), apiMux)
	}()

	// Main proxy
	proxyMux := http.NewServeMux()
	mr := chi.NewRouter()
	// Chain HTTP Middleware
	mr.Use(middleware.Heartbeat("/ping"))
	mr.Use(middleware.RequestID)
	mr.Use(middleware.Recoverer)
	mr.HandleFunc("/*", s.Proxy)
	proxyMux.Handle("/", mr)

	log.WithFields(log.Fields{
		"host": s.cfg.Host,
		"port": s.cfg.Port,
	}).Info("Proxy HTTP server starting")
	go func() {
		errc <- http.ListenAndServe(fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port), proxyMux)
	}()

	log.Fatal(<-errc)
}

// Proxy requests via Toxiproxy proxies or the upstream server if no match.
func (s *ShrikeServer) Proxy(w http.ResponseWriter, req *http.Request) {
	// Either a proxy on the Toxy or the vanilla upstream address.
	if u, m := s.ProxyStore.Match(req.URL.Path); m {
		req.URL = &u
	} else {
		req.URL = s.upstream
	}
	s.fwd.ServeHTTP(w, req)
}

// GetProxies gets proxies from Toxiproxy and maps with the routes we match from.
func (s *ShrikeServer) GetProxies(w http.ResponseWriter, req *http.Request) {
	proxies, err := s.client.Proxies()
	if err != nil {
		log.WithField("err", err).Error("Error getting proxies")
		RespondWithError(w, http.StatusInternalServerError, JSONError{
			Status:  "Server Error",
			Message: "Could not create the path entry.",
		})
		return
	}

	proxyEntries := s.ProxyStore.ToMap()
	routeMap := map[string]RouteWithProxy{}
	for k := range proxyEntries {
		toxy := proxies[routes.ProxyNameFrom(s.cfg.ToxyNamePathSeparator, k)]
		if toxy == nil {
			log.WithField("path", k).Warn("No proxy entry found in Toxiproxy.")
			continue
		}
		routeMap[k] = RouteWithProxy{
			Route: Route{
				Prefix: k,
			},
			Toxy: toxy,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(routeMap)
	w.Write(b)
}

// AddProxy cfg to Toxiproxy.
func (s *ShrikeServer) AddProxy(w http.ResponseWriter, req *http.Request) {
	// Decode payload into a Route
	defer req.Body.Close()
	body, _ := ioutil.ReadAll(req.Body)
	doc := &Route{}
	if err := json.Unmarshal(body, &doc); err != nil || doc.Prefix == "" {
		log.Errorf("Error unmarshaling body %s", err)
		RespondWithError(w, http.StatusBadRequest, JSONError{
			Status:  "Bad Request",
			Message: "Request body is not a valid JSON Route object.",
		})
		return
	}
	proxyName := routes.ProxyNameFrom(s.cfg.ToxyNamePathSeparator, doc.Prefix)
	proxy, err := s.client.CreateProxy(
		proxyName,
		fmt.Sprintf("%s:%d", s.cfg.ToxyAddress, routes.NumFrom(proxyName)),
		s.upstream.Host,
	)
	if err != nil {
		proxy, err = s.client.Proxy(proxyName)
		if err != nil {
			log.WithField("err", err).Error("Error creating/getting a proxy")
			RespondWithError(w, http.StatusInternalServerError, JSONError{
				Status:  "Server Error",
				Message: "Could not create the path entry.",
			})
			return
		}
	}

	s.ProxyStore.Add(proxy)

	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(proxy)
	w.Write(b)
}

// GetRoute gets proxies from Toxiproxy and maps with the routes we match from.
func (s *ShrikeServer) GetRoute(w http.ResponseWriter, req *http.Request) {
	route := chi.URLParam(req, "route")
	if route == "" {
		log.WithField("Route", route).Info("Route must be the name of one of the proxy paths.")
		RespondWithError(w, http.StatusBadRequest, JSONError{
			Status:  "Bad Request",
			Message: "Route must be the name of one of the proxy paths.",
		})
		return
	}

	toxy, err := s.client.Proxy(route)
	if err != nil {
		log.WithFields(log.Fields{
			"Route": route,
			"err":   err,
		}).Info("Error getting proxy from toxiproxy")
		RespondWithError(w, http.StatusNotFound, JSONError{
			Status:  "No Proxy",
			Message: "No proxy by that name.",
		})
		return
	}

	if p := s.ProxyStore.Get(routes.PathNameFrom(s.cfg.ToxyNamePathSeparator, route)); p == nil {
		log.WithFields(log.Fields{
			"Route": route,
			"err":   err,
		}).Info("Error getting proxy from the store")
		RespondWithError(w, http.StatusNotFound, JSONError{
			Status:  "No Proxy",
			Message: "No proxy by that name.",
		})
		return
	}

	b, _ := json.Marshal(RouteWithProxy{
		Route: Route{
			Prefix: routes.PathNameFrom(s.cfg.ToxyNamePathSeparator, route),
		},
		Toxy: toxy,
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// DeleteRoute cfg removes a route from the proxy
func (s *ShrikeServer) DeleteRoute(w http.ResponseWriter, req *http.Request) {
	route := chi.URLParam(req, "route")
	if route == "" {
		log.WithField("Route", route).Info("Route must be the name of one of the proxy paths.")
		RespondWithError(w, http.StatusBadRequest, JSONError{
			Status:  "Bad Request",
			Message: "Route must be the name of one of the proxy paths.",
		})
		return
	}

	proxy, err := s.client.Proxy(route)
	if err != nil {
		log.WithFields(log.Fields{
			"Route": route,
			"err":   err,
		}).Info("Error getting proxy to delete")
		RespondWithError(w, http.StatusGone, JSONError{
			Status:  "No Proxy",
			Message: "No proxy by that name to remove.",
		})
		return
	}
	err = proxy.Delete()
	if err != nil {
		log.WithFields(log.Fields{
			"Route": route,
			"err":   err,
		}).Error("Error deleting a proxy")
		RespondWithError(w, http.StatusInternalServerError, JSONError{
			Status:  "Server Error",
			Message: "Could not delete the proxy.",
		})
		return
	}

	s.ProxyStore.Delete(proxy)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

// RespondWithError writes the error to the response writer
func RespondWithError(w http.ResponseWriter, s int, j JSONError) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(s)
	w.Write(j.Bytes())
}
