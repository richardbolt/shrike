package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"shrike/routes"

	toxy "github.com/Shopify/toxiproxy/client"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	log "github.com/sirupsen/logrus"
)

// New API mux
func New(sep, toxyAddress string, client *toxy.Client) *http.ServeMux {
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

	r.Post("/routes", func(w http.ResponseWriter, req *http.Request) {
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
		//
		proxyName := routes.ProxyNameFrom(sep, doc.Path)
		proxy, err := client.CreateProxy(
			proxyName,
			fmt.Sprintf("%s:%d", toxyAddress, routes.NumFrom(proxyName)),
			"localhost:8000", // TODO: Get the proper upstream host into here.
		)
		if err != nil {
			proxy, err = client.Proxy(proxyName)
			if err != nil {
				log.Errorf("Error creating/getting a proxy %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"status": "Server Error", "message": "Could not create the path entry."}`))
				return
			}
		}

		b, _ := json.Marshal(proxy)
		w.Write(b)
	})

	mux.Handle("/", r)
	return mux
}

// Route holds information about the routing of this request
type Route struct {
	Path string `json:"path"`
}
