HAL
===

HAL is a Layer 7 HTTP/WebSocket proxy designed to sit in front of both a downstream server and a [Toxiproxy](https://github.com/Shopify/toxiproxy) Layer 4 network tampering tool. HAL routes explicitly tampered traffic through the Toxiproxy server to the downstream server.

HAL has an API to route http path based traffic through a [Toxiproxy](https://github.com/Shopify/toxiproxy) server for resliliency testing and on to the downstream server. The HAL API abstracts the Toxiproxy API seamlessly.

Develop
-------

This project uses [Go 1.8](https://golang.org/dl/) or later and uses [Glide](https://glide.sh/) for package management.

### Linux
```
make
```

### Mac
```
make mac
```

To add or update dependencies in the vendor folder, please use [Glide](https://glide.sh/):

```
curl https://glide.sh/get | sh # only necessary if you don't have Glide installed already.
glide install
```

Testing
-------

Run the test suites with [Ginkgo](http://onsi.github.io/ginkgo/) installed and get coverage output:

```
make ginkgo
```

Run the test suites without Ginkgo (less awesome output, no randomization of tests):

```
make test
```

Environment Variables
---------------------

`PORT` is the listen port to bind to on the host. Defaults to `8080`.

`API_PORT` is the listen listen port to bind to on the host. Defaults to `8075`.

`TOXY_ADDRESS` is the IP or DNS address or your [Toxiproxy](https://github.com/Shopify/toxiproxy) server. Defaults to `"127.0.0.1"`.

`DOWNSTREAM_PROXY_URL` is the downstream HTTP/WS proxy we are sitting in front of. Defaults to `http://127.0.0.1`.

Full config in `cfg/env.go`.