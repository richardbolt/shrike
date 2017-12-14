The Shrike
==========

The Shrike is a Layer 7 Chaos HTTP/WebSocket proxy that impales it's victims on the Tree of Pain. The Tree of Pain for this Shrike takes the form of [Toxiproxy](http://toxiproxy.io), an excellent TCP network tampering tool.

The Shrike is designed for resliliency testing a whole environment and has an API to route http path based traffic through an embedded [Toxiproxy](http://toxiproxy.io) instance.

The Shrike currently assumes you've put it close at the edge of your service stack thus the current single upstream location which is assumed to be your gateway.

Path prefix matching is used to route differing paths for testing while unmatched paths are sent straight on to the gateway.

Configuration
-------------

Configuration is by both environment variables and command line flags with command line flags taking precedence.

### Command line flags

`-host` is the address to bind to on the host. Defaults to `0.0.0.0`.

`-port` is the proxy forwarder listen port to bind to on the host. Defaults to `8080`.

`-apiport` is the api listen port to bind to on the host. Defaults to `8075`.

`-upstream` is the upstream HTTP/WS proxy we are sitting in front of. Defaults to `http://127.0.0.1`.


### Environment Variables

`HOST` is the address to bind to on the host. Defaults to `0.0.0.0`.

`PORT` is the proxy forwarder listen port to bind to on the host. Defaults to `8080`.

`API_PORT` is the api listen port to bind to on the host. Defaults to `8075`.

`UPSTREAM_URL` is the upstream HTTP/WS proxy we are sitting in front of. Defaults to `http://127.0.0.1`.

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

