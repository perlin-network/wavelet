package wctl

import (
	"fmt"
	"net/url"
	"time"

	"github.com/valyala/fastjson"
)

// PollTransactions calls the callback for each WS event received. On error, the
// callback may be called twice.
func (c *Client) PollTransactions(callback func([]TransactionEvent, error),
	txID string, senderID string, creatorID string, tag *byte) (func(), error) {

	v := url.Values{}

	if txID != "" {
		v.Set("tx_id", txID)
	}

	if senderID != "" {
		v.Set("sender", senderID)
	}

	if creatorID != "" {
		v.Set("creator", creatorID)
	}

	if tag != nil {
		v.Set("tag", fmt.Sprintf("%x", *tag))
	}

	return c.pollWS(RouteWSTransactions, v, func(b []byte) {
		var parser fastjson.Parser
		v, err := parser.ParseBytes(b)
		if err != nil {
			callback(nil, err)
			return
		}

		a := v.GetArray()
		txs := make([]TransactionEvent, 0, len(a))

		for _, o := range a {
			var t TransactionEvent

			if err := jsonHex(o, t.ID[:], "tx_id"); err != nil {
				callback(nil, errUnmarshallingFail(b, "tx_id", err))
				continue
			}

			if err := jsonHex(o, t.Sender[:], "sender_id"); err != nil {
				callback(nil, errUnmarshallingFail(b, "sender_id", err))
				continue
			}

			if err := jsonHex(o, t.Creator[:], "creator_id"); err != nil {
				callback(nil, errUnmarshallingFail(b, "creator_id", err))
				continue
			}

			t.Event = string(o.GetStringBytes("event"))
			t.Depth = o.GetUint64("depth")
			t.Tag = byte(o.GetUint("tag"))

			Time, err := time.Parse(
				time.RFC3339, string(o.GetStringBytes("time")),
			)

			if err != nil {
				callback(nil, errUnmarshallingFail(b, "time", err))
				continue
			}

			t.Time = Time

			txs = append(txs, t)
		}

		if len(txs) > 0 {
			callback(txs, nil)
		}
	})
}
