//go:generate protoc -I. -I$GOPATH/src --gogofast_out=. node/messages.proto
//go:generate protoc -I. --gogofast_out=. iavl/node.proto
//go:generate protoc -I. --gogofast_out=. delta.proto

package wavelet
