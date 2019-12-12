BINOUT = $(shell pwd)/build

R = localhost:5000
T = latest
G = $(shell git rev-parse --short HEAD)

protoc:
	protoc --gogofaster_out=plugins=grpc:. -I=. rpc.proto

protoc-docker:
	docker run --rm -v `pwd`:/src znly/protoc --gogofaster_out=plugins=grpc:. -I=. src/rpc.proto

integration_test:
	go test -tags=integration -v -coverprofile=coverage_integration.txt -covermode=atomic -timeout=60m -parallel 1 ./...

unit_test:
	go test -tags=unit -v -coverprofile=coverage_unit.txt -covermode=atomic -race ./...

test: unit_test integration_test

fmt:
	go fmt ./...

lint:
#	https://github.com/golangci/golangci-lint#install
	golangci-lint -c .golangci.yml run

check: fmt lint test

bench:
	go test -bench=. -benchmem

upload:
	cd cmd/graph && env GOOS=linux GOARCH=amd64 go build -o main
	rsync -avz cmd/graph/main root@104.248.44.250:/root

docker:
	docker build --build-arg=GIT_COMMIT=$(G) -t wavelet:$(T) .
ifneq ($(R),)
	docker tag wavelet:$(T) $(R)/wavelet:$(T)
	docker push $(R)/wavelet:$(T)
endif

docker_aws:
	$(shell aws ecr get-login --no-include-email)
	$(MAKE) docker 'R=010313437810.dkr.ecr.us-east-2.amazonaws.com/perlin' 'T=$(T)'

docker_hub:
	$(MAKE) docker 'R=perlin' 'T=$(T)'

clean:
	rm -rf $(BINOUT)

build-all: linux windows darwin linux-arm64
	@echo "Done building all targets."

release: clean build-all
	scripts/release.sh

linux:
	scripts/build.sh -a linux-amd64

windows:
	scripts/build.sh -a windows-amd64

darwin:
	scripts/build.sh -a darwin-amd64

linux-arm64:
	scripts/build.sh -a linux-arm64

license:
	addlicense -l mit -c Perlin $(PWD)

.PHONY: protoc-docker test bench upload docker docker_aws docker_hub clean build-all release linux windows darwin linux-arm64 license
