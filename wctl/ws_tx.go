package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"

	"github.com/valyala/fastjson"
)

// PollTransactions calls the callback for each WS event received. On error, the
// callback may be called twice.
func (c *Client) PollTransactions() (func(), error) {
	return c.pollWS(RouteWSTransactions, func(v *fastjson.Value) error {
		var (
			tx = &wavelet.Transaction{}
			ev = log.ValueString(v, "event")
		)

		switch ev {
		case "applied", "rejected":
			// noop
		default:
			return errInvalidEvent(v, ev)
		}

		if err := tx.UnmarshalValue(v); err != nil {
			return err
		}

		switch ev {
		case "applied":
			tx := &wavelet.TxApplied{
				Transaction: tx,
			}

			for _, v := range c.handlers {
				switch f := v.(type) {
				case func(log.MarshalableEvent):
					f(tx)
				case func(*wavelet.TxApplied):
					f(tx)
				}
			}
		case "rejected":
			tx := &wavelet.TxRejected{
				Transaction: tx,
			}

			for _, v := range c.handlers {
				switch f := v.(type) {
				case func(log.MarshalableEvent):
					f(tx)
				case func(*wavelet.TxRejected):
					f(tx)
				}
			}
		}

		return nil
	})
}
