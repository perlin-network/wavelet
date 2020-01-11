package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

func (c *Client) PollAccounts() (func(), error) {
<<<<<<< HEAD
	return c.pollWSArray(RouteWSAccounts, func(v *fastjson.Value) error {
		var (
			a  log.Loggable
			ev = log.ValueString(v, "event")
		)

		switch ev {
		case "balance_updated":
			a = &wavelet.AccountBalanceUpdated{}
		case "gas_balance_updated":
			a = &wavelet.AccountGasBalanceUpdated{}
		case "num_pages_updated":
			a = &wavelet.AccountNumPagesUpdated{}
		case "stake_updated":
			a = &wavelet.AccountStakeUpdated{}
		case "reward_updated":
			a = &wavelet.AccountRewardUpdated{}
		default:
			return errInvalidEvent(v, ev)
		}

		if err := a.UnmarshalValue(v); err != nil {
			return err
		}

		for _, v := range c.handlers {
			if f, ok := v.(func(log.MarshalableEvent)); ok {
				f(a)
				continue
			}

			switch a := a.(type) {
			case *wavelet.AccountBalanceUpdated:
				if f, ok := v.(func(*wavelet.AccountBalanceUpdated)); ok {
					f(a)
				}
			case *wavelet.AccountGasBalanceUpdated:
				if f, ok := v.(func(*wavelet.AccountGasBalanceUpdated)); ok {
					f(a)
				}
			case *wavelet.AccountNumPagesUpdated:
				if f, ok := v.(func(*wavelet.AccountNumPagesUpdated)); ok {
					f(a)
				}
			case *wavelet.AccountRewardUpdated:
				if f, ok := v.(func(*wavelet.AccountRewardUpdated)); ok {
					f(a)
				}
			case *wavelet.AccountStakeUpdated:
				if f, ok := v.(func(*wavelet.AccountStakeUpdated)); ok {
					f(a)
				}
=======
	return c.pollWS(RouteWSAccounts, func(o *fastjson.Value) {
		var err error

		if err := checkMod(o, "accounts"); err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
			return
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
		default:
			err = errInvalidEvent(o, ev)
		}

		if err != nil {
			if c.OnError != nil {
				c.OnError(err)
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
			}
		}

<<<<<<< HEAD
		return nil
	})
=======
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
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
}
