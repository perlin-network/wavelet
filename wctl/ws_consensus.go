package wctl

import (
	"sync/atomic"

	"github.com/valyala/fastjson"
)

func (c *Client) pollConsensus() (func(), error) {
	return c.pollWS(RouteWSConsensus, func(v *fastjson.Value) {
		var err error

		if err := checkMod(v, "consensus"); err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
			return
		}

		switch ev := jsonString(v, "event"); ev {
		case "pull-transactions":
			// err = parseConsensusProposal(c, v)
		case "proposal":
			err = parseConsensusProposal(c, v)
		case "finalized":
			err = parseConsensusFinalized(c, v)
		default:
			err = errInvalidEvent(v, ev)
		}

		if err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
		}
	})
}

/* TODO
func parseConsensusPullTxs(c *Client, v *fastjson.Value) error {

}
*/

func parseConsensusProposal(c *Client, v *fastjson.Value) error {
	var p Proposal

	if err := jsonHex(v, p.BlockID[:], "block_id"); err != nil {
		return err
	}

	p.BlockIndex = v.GetUint64("block_index")
	p.NumTxs = v.GetUint64("num_transactions")
	p.Message = string(v.GetStringBytes("message"))

	if c.OnProposal != nil {
		c.OnProposal(p)
	}
	return nil
}

func parseConsensusFinalized(c *Client, v *fastjson.Value) error {
	var f Finalized

	if err := jsonHex(v, f.BlockID[:], "new_block_id"); err != nil {
		return err
	}

	f.BlockHeight = v.GetUint64("new_block_height")
	f.Message = string(v.GetStringBytes("message"))

	atomic.StoreUint64(&c.Block, f.BlockHeight)

	if c.OnFinalized != nil {
		c.OnFinalized(f)
	}
	return nil
}
