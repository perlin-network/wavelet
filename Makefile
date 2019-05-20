.PHONY: wavelet all build-all docker docker_aws test bench clean
.PHONY: linux
.PHONY: windows
.PHONY: darwin
.PHONY: linux-arm

BINOUT = $(shell pwd)/build
WAVELET_DIR = $(shell pwd)/cmd/wavelet

all: build-all test bench

build-all: linux windows darwin linux-arm64
	@echo "Done building all targets."

docker:
	docker build -t wavelet .
	docker tag wavelet:latest localhost:5000/wavelet
	docker push localhost:5000/wavelet

docker_aws:
	$(shell aws ecr get-login --no-include-email)
	docker build -t wavelet .
	docker tag wavelet:latest 010313437810.dkr.ecr.us-east-2.amazonaws.com/perlin/wavelet
	docker push 010313437810.dkr.ecr.us-east-2.amazonaws.com/perlin/wavelet

docker_hub:
	docker build -t wavelet .
	docker tag wavelet:latest repo.treescale.com/perlin/wavelet
	docker push repo.treescale.com/perlin/wavelet

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

protoc:
	docker run --rm -v `pwd`/proto:/proto -v `pwd`/pb:/pb znly/protoc --gogofaster_out=plugins=grpc:. -I=. proto/wavelet.proto

wavelet:
	go run $(WAVELET_DIR)/main.go --config $(WAVELET_DIR)/config/config.toml

bench:
	go test -bench=. -benchmem

clean:
	rm -rf $(BINOUT)

release: clean build-all
	scripts/release.sh
