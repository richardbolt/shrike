# Default to linux, for build boxes/prod.
GOOS?=linux
GOARCH?=amd64
CGO_ENABLED?=1
all: test linux

install:
	glide install

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o bin/server cmd/main.go

linux: CGO_ENABLED=0
linux: test build

mac: GOOS = darwin
mac: test build

run: all
	./bin/server

run_mac: mac
	./bin/server

test:
	go test -race -cover `go list ./... | grep -v /vendor/`

ginkgo:
	ginkgo -r \
		--randomizeAllSpecs \
		--randomizeSuites \
		--failOnPending \
		--cover \
		--trace \
		--race \
		--progress \
		--skipPackage ./vendor

clean:
	rm -rf bin/server