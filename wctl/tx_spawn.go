package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) Spawn(code []byte, gasLimit uint64) (*TxResponse, error) {
	ct := wavelet.Contract{
		GasLimit: 100000000,
		Code:     code,
	}

	if gasLimit > 0 {
		ct.GasLimit = gasLimit
	}

	return c.sendTransfer(byte(sys.TagContract), ct)
}
