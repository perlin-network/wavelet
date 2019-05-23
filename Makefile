protoc:
	protoc --gogofaster_out=plugins=grpc:. -I=. rpc.proto
	protoc --gogofaster_out=plugins=grpc:. -I=. cmd/graph/rpc.proto

upload:
	cd cmd/graph && go build -o main
	rsync -avz cmd/graph/main root@104.248.44.250:/root