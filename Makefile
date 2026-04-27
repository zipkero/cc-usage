BINARY := cc-usage
VERSION := 0.1.4
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 windows/amd64

.PHONY: build build-local test clean

build:
	@for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} \
		go build -ldflags="-s -w -X main.version=$(VERSION)" \
		-o bin/$(BINARY)-$${p%/*}-$${p#*/}$$([ "$${p%/*}" = "windows" ] && echo ".exe") .; \
	done
	@chmod +x bin/run.sh bin/$(BINARY)-darwin-* bin/$(BINARY)-linux-*
	@git update-index --chmod=+x bin/run.sh bin/$(BINARY)-darwin-* bin/$(BINARY)-linux-* 2>/dev/null || true

build-local:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY) .

test:
	go test ./...

clean:
	rm -rf bin/ dist/
