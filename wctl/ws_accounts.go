package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

func (c *Client) PollAccounts() (func(), error) {
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
			}
		}

		return nil
	})
}
