//go:generate protoc -I. -I$GOPATH/src --gogofast_out=. node/messages.proto
//go:generate protoc -I. --gogofast_out=. iavl/node.proto
//go:generate protoc -I. --gogofast_out=. delta.proto

//go:generate mockgen -destination=ledger_mock.go -package=wavelet github.com/perlin-network/wavelet LedgerInterface
//go:generate mockgen -destination=node/plugin_mock.go -package=node github.com/perlin-network/wavelet/node NodeInterface

package wavelet
