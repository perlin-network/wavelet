//go:generate protoc -I. --gogofast_out=. tree/node.proto
//go:generate protoc -I. --gogofast_out=. delta.proto

package wavelet
