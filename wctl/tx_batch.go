package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) SendBatch(batch wavelet.Batch) (*api.TxResponse, error) {
	b, err := batch.Marshal()
	if err != nil {
		return nil, err
	}

	return c.SendTransaction(byte(sys.TagBatch), b)
}
