package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) SendBatch(batch wavelet.Batch) (*api.TxResponse, error) {
	return c.SendTransaction(byte(sys.TagBatch), batch.Marshal())
}
