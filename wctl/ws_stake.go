package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollStake() (func(), error) {
	return c.pollWS(RouteWSStake, func(v *fastjson.Value) {
		var err error

		for _, o := range v.GetArray() {
			if err := checkMod(o, "stake"); err != nil {
				c.OnError(err)
				continue
			}

			switch ev := jsonString(o, "event"); ev {
			case "reward_validator":
				err = parseRewardValidator(c, o)
			default:
				err = errInvalidEvent(o, ev)
			}

			if err != nil {
				c.OnError(err)
			}
		}
	})
}

func parseRewardValidator(c *Client, v *fastjson.Value) error {
	var r StakeRewardValidator

	if err := jsonHex(v, r.Creator[:], "creator"); err != nil {
		return err
	}

	if err := jsonHex(v, r.Recipient[:], "recipient"); err != nil {
		return err
	}

	if err := jsonHex(v, r.CreatorTxID[:], "creator_tx_id"); err != nil {
		return err
	}

	if err := jsonHex(v, r.RewardeeTxID[:], "rewardee_tx_id"); err != nil {
		return err
	}

	if err := jsonHex(v, r.Entropy[:], "entropy"); err != nil {
		return err
	}

	r.Accuracy = v.GetFloat64("acc")
	r.Threshold = v.GetFloat64("threshold")
	r.Message = string(v.GetStringBytes("message"))

	c.OnStakeRewardValidator(r)
	return nil
}
