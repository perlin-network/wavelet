BINOUT = $(shell pwd)/build

protoc:
	protoc --gogofaster_out=plugins=grpc:. -I=. rpc.proto

test:
	go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...

bench:
	go test -bench=. -benchmem

upload:
	cd cmd/graph && env GOOS=linux GOARCH=amd64 go build -o main
	rsync -avz cmd/graph/main root@104.248.44.250:/root

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