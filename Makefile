.PHONY: wavelet all build-all test bench clean
.PHONY: linux
.PHONY: windows
.PHONY: darwin
.PHONY: linux-arm

BINOUT = $(shell pwd)/build
WAVELET_DIR = $(shell pwd)/cmd/wavelet

all: build-all test bench

build-all: linux windows darwin linux-arm64
	@echo "Done building all targets."

linux:
	scripts/build.sh -a linux-amd64

windows:
	scripts/build.sh -a windows-amd64

darwin:
	scripts/build.sh -a darwin-amd64

linux-arm64:
	scripts/build.sh -a linux-arm64

test:
	go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...

wavelet:
	go run $(WAVELET_DIR)/main.go --config $(WAVELET_DIR)/config/config.toml

bench:
	go test -bench=. -benchmem

clean:
	rm -rf $(BINOUT)

release: clean build-all
	scripts/release.sh
