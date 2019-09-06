package wctl

import (
	"github.com/valyala/fastjson"
)

// PollTransactions calls the callback for each WS event received. On error, the
// callback may be called twice.
func (c *Client) PollTransactions() (func(), error) {
	return c.pollWS(RouteWSTransactions, func(b []byte) {
		var parser fastjson.Parser
		v, err := parser.ParseBytes(b)
		if err != nil {
			c.OnError(err)
			return
		}

		if string(v.GetStringBytes("mod")) != "tx" {
			c.OnError(errMismatchMod(v, "tx"))
		}

		for _, o := range v.GetArray() {
			var err error

			switch ev := jsonString(o, "event"); {
			case ev == "applied":
				err = parseTxApplied(c, o)
			case ev == "gossip" && jsonString(o, "level") == "error":
				err = parseTxGossipError(c, o)
			case ev == "failed":
				err = parseTxFailed(c, o)
			}

			if err != nil {
				c.OnError(err)
			}
		}
	})
}

// parse<mod><event>

func parseTxApplied(c *Client, v *fastjson.Value) error {
	var t TxApplied

	if err := jsonHex(v, t.TxID[:], "tx_id"); err != nil {
		return errUnmarshallingFail(v, "tx_id", err)
	}

	if err := jsonHex(v, t.SenderID[:], "sender_id"); err != nil {
		return errUnmarshallingFail(v, "sender_id", err)
	}

	if err := jsonHex(v, t.CreatorID[:], "creator_id"); err != nil {
		return errUnmarshallingFail(v, "creator_id", err)
	}

	t.Depth = v.GetUint64("depth")
	t.Tag = byte(v.GetUint("tag"))

	if err := jsonTime(v, &t.Time, "time"); err != nil {
		return errUnmarshallingFail(v, "time", err)
	}

	c.OnTxApplied(t)

	return nil
}

func parseTxGossipError(c *Client, v *fastjson.Value) error {
	var t TxGossipError

	if err := jsonTime(v, &t.Time, "time"); err != nil {
		return errUnmarshallingFail(v, "time", err)
	}

	t.Error = jsonString(v, "error")
	t.Message = jsonString(v, "message")

	c.OnTxGossipError(t)

	return nil
}

func parseTxFailed(c *Client, v *fastjson.Value) error {
	var t TxFailed

	if err := jsonHex(v, t.TxID[:], "tx_id"); err != nil {
		return errUnmarshallingFail(v, "tx_id", err)
	}

	if err := jsonHex(v, t.SenderID[:], "sender_id"); err != nil {
		return errUnmarshallingFail(v, "sender_id", err)
	}

	if err := jsonHex(v, t.CreatorID[:], "creator_id"); err != nil {
		return errUnmarshallingFail(v, "creator_id", err)
	}

	t.Depth = v.GetUint64("depth")
	t.Tag = byte(v.GetUint("tag"))
	t.Error = jsonString(v, "error")

	if err := jsonTime(v, &t.Time, "time"); err != nil {
		return errUnmarshallingFail(v, "time", err)
	}

	c.OnTxFailed(t)

	return nil
}
