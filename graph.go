package wavelet

import "golang.org/x/crypto/blake2b"

type Graph struct {
	transactions map[[blake2b.Size256]byte]*Transaction
}

func NewGraph() *Graph {
	return &Graph{
		transactions: make(map[[blake2b.Size256]byte]*Transaction),
	}
}
