//go:generate protoc -I. --gogofast_out=. iavl/node.proto
//go:generate protoc -I. --gogofast_out=. delta.proto

package wavelet
