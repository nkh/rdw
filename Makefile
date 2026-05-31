BINARY     := rdw
MODULE     := github.com/nkh/rdw
GOFLAGS    := -mod=readonly
PROXY      := file:///tmp/goproxy,off
GONOSUMDB  := *

.PHONY: all build test vet lint clean coverage selftest

all: vet test build

build:
	GONOSUMDB=$(GONOSUMDB) GOPROXY=$(PROXY) GOFLAGS=$(GOFLAGS) \
		go build -o $(BINARY) ./cmd/rdw

test:
	GONOSUMDB=$(GONOSUMDB) GOPROXY=$(PROXY) GOFLAGS=$(GOFLAGS) \
		go test -race -count=1 ./...

vet:
	GONOSUMDB=$(GONOSUMDB) GOPROXY=$(PROXY) GOFLAGS=$(GOFLAGS) \
		go vet ./...

coverage:
	GONOSUMDB=$(GONOSUMDB) GOPROXY=$(PROXY) GOFLAGS=$(GOFLAGS) \
		go test -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -func=coverage.txt

selftest: build
	./$(BINARY) selftest

clean:
	rm -f $(BINARY) coverage.txt
