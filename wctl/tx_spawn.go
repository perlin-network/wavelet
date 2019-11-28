package wctl

import (
	wasm "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) Spawn(code []byte, gasLimit uint64) (*api.TxResponse, error) {
	ct := wavelet.Contract{
		GasLimit: 100000000,
		Code:     code,
	}

	if gasLimit > 0 {
		ct.GasLimit = gasLimit
	}

	if err := wasm.GetValidator().ValidateWasm(code); err != nil {
		return nil, err
	}

	return c.sendTransfer(byte(sys.TagContract), ct)
}
