.PHONY: wavelet all build-all test bench clean
.PHONY: linux
.PHONY: windows

BINOUT = $(shell pwd)/build/bin
WAVELET_DIR = $(shell pwd)/cmd/wavelet

all: build-all test bench

build-all: linux windows
	@echo "Done building all targets."

linux:
	build/build.sh -a linux-amd64

windows:
	build/build.sh -a windows-amd64

test:
	go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...

wavelet:
	go run $(WAVELET_DIR)/main.go -config $(WAVELET_DIR)/config.toml -genesis $(WAVELET_DIR)/genesis.json

bench:
	go test -bench=. -benchmem

clean:
	rm -rf $(BINOUT)