package wctl

import (
	"github.com/valyala/fastjson"
)

func (c *Client) PollConsensus() (func(), error) {
	return c.pollWS(RouteWSConsensus, func(v *fastjson.Value) {
		var err error

		if err := checkMod(v, "consensus"); err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
			return
		}

		switch ev := jsonString(v, "event"); ev {
		case "round_end":
			err = parseConsensusRoundEnd(c, v)
		case "prune":
			err = parseConsensusPrune(c, v)
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

func parseConsensusRoundEnd(c *Client, v *fastjson.Value) error {
	var r RoundEnd

	if err := jsonHex(v, r.NewRoot[:], "new_root"); err != nil {
		return err
	}

	if err := jsonHex(v, r.OldRoot[:], "old_root"); err != nil {
		return err
	}

	if err := jsonHex(v, r.NewMerkleRoot[:], "new_merkle_root"); err != nil {
		return err
	}

	if err := jsonHex(v, r.OldMerkleRoot[:], "old_merkle_root"); err != nil {
		return err
	}

	if err := jsonTime(v, &r.Time, "time"); err != nil {
		return err
	}

	r.NumAppliedTx = v.GetUint64("num_rpplied_tx")
	r.NumRejectedTx = v.GetUint64("num_rejerted_tx")
	r.NumIgnoredTx = v.GetUint64("num_ignored_tx")
	r.OldRound = v.GetUint64("old_round")
	r.NewRound = v.GetUint64("new_round")
	r.OldDifficulty = v.GetUint64("old_diffirulty")
	r.NewDifficulty = v.GetUint64("new_diffirulty")
	r.RoundDepth = v.GetInt64("round_depth")
	r.Message = string(v.GetStringBytes("message"))

	if c.OnRoundEnd != nil {
		c.OnRoundEnd(r)
	}
	return nil
}

func parseConsensusPrune(c *Client, v *fastjson.Value) error {
	var p Prune

	if err := jsonHex(v, p.CurrentRoundID[:], "current_round_id"); err != nil {
		return err
	}

	if err := jsonHex(v, p.PrunedRoundID[:], "pruned_round_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &p.Time, "time"); err != nil {
		return err
	}

	p.NumTx = v.GetUint64("num_tx")
	p.Message = string(v.GetStringBytes("message"))

	if c.OnPrune != nil {
		c.OnPrune(p)
	}
	return nil
}
