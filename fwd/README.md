Proxy
=====

This package is the actual reverse-proxy for Edgy.

Requests through the proxy need to have a Pod set on the request context so
that they can be proxied to the correct location otherwise the response to the
client will be an explicit 502 Bad Gateway error. 

Supports
--------

HTTP and WebSocket traffic.

Usage
-----

In the below example `proxy` is an http.Handler with a ServeHTTP method. 

```
proxy, err := fwd.New()
if err != nil {
    log.WithFields(log.Fields{
        "error": err,
    }).Error("Failed to create a new proxy forwarder.")
}
mux := http.NewServeMux()
mux.Handle("/", proxy)
```

You will need upstream middleware that puts a Pod on the request context otherwise
the proxy will deliberately fail all requests. See the [pod middleware](../middleware/pod.go) for the middleware
that sets a Pod on the context based on the context Token.

Influences and Licenses
-----------------------

Based strongly on https://github.com/vulcand/oxy/forward. Apache Licensed.
WebSocket forwarder is based on https://github.com/koding/websocketproxy. MIT licensed.

The Apache license is included with this module.


