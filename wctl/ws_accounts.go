package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollAccounts() (func(), error) {
	return c.pollWS(RouteWSAccounts, func(v *fastjson.Value) {
		var err error

		for _, o := range v.GetArray() {
			if err := checkMod(o, "accounts"); err != nil {
				if c.OnError != nil {
					c.OnError(err)
				}
				continue
			}

			switch ev := jsonString(o, "event"); ev {
			case "balance_updated":
				err = parseAccountsBalanceUpdated(c, o)
			case "gas_balance_updated":
				err = parseAccountsGasBalanceUpdated(c, o)
			case "num_pages_updated":
				err = parseAccountNumPagesUpdated(c, o)
			case "stake_updated":
				err = parseAccountStakeUpdated(c, o)
			case "reward_updated":
				err = parseAccountRewardUpdated(c, o)
			case "nonce_updated":
				err = parseAccountNonceUpdated(c, o)
			default:
				err = errInvalidEvent(o, ev)
			}

			if err != nil {
				if c.OnError != nil {
					c.OnError(err)
				}
			}
		}
	})
}

func parseAccountsBalanceUpdated(c *Client, v *fastjson.Value) error {
	var a BalanceUpdate

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.Balance = v.GetUint64("balance")

	if c.OnBalanceUpdated != nil {
		c.OnBalanceUpdated(a)
	}

	return nil
}

func parseAccountsGasBalanceUpdated(c *Client, v *fastjson.Value) error {
	var a GasBalanceUpdate

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.GasBalance = v.GetUint64("gas_balance")

	if c.OnGasBalanceUpdated != nil {
		c.OnGasBalanceUpdated(a)
	}

	return nil
}

func parseAccountNumPagesUpdated(c *Client, v *fastjson.Value) error {
	var a NumPagesUpdated

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.NumPages = v.GetUint64("num_pages_updated")

	if c.OnNumPagesUpdated != nil {
		c.OnNumPagesUpdated(a)
	}

	return nil
}

func parseAccountStakeUpdated(c *Client, v *fastjson.Value) error {
	var a StakeUpdated

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.Stake = v.GetUint64("stake")

	if c.OnStakeUpdated != nil {
		c.OnStakeUpdated(a)
	}

	return nil
}

func parseAccountRewardUpdated(c *Client, v *fastjson.Value) error {
	var a RewardUpdated

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.Reward = v.GetUint64("reward")

	if c.OnRewardUpdated != nil {
		c.OnRewardUpdated(a)
	}

	return nil
}

func parseAccountNonceUpdated(c *Client, v *fastjson.Value) error {
	var a NonceUpdated

	if err := jsonHex(v, a.AccountID[:], "account_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &a.Time, "time"); err != nil {
		return err
	}

	a.Nonce = v.GetUint64("nonce")

	if c.OnNonceUpdated != nil {
		c.OnNonceUpdated(a)
	}
	return nil
}
