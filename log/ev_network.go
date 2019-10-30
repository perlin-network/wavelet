package log

import "github.com/perlin-network/wavelet"

type NetworkJoined struct {
	PublicKey wavelet.AccountID
	Address   string
}

// TODO
var _ Loggable = (*NetworkJoined)(nil)

type NetworkLeft struct {
	PublicKey wavelet.AccountID
	Address   string
}

var _ Loggable = (*NetworkLeft)(nil)
