package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollNetwork() (func(), error) {
	return c.pollWS(RouteWSNetwork, func(v *fastjson.Value) {
		var err error

		if err := checkMod(v, "network"); err != nil {
			c.OnError(err)
			return
		}

		switch ev := jsonString(v, "event"); ev {
		case "joined":
			err = parsePeerJoin(c, v)
		case "left":
			err = parsePeerLeave(c, v)
		default:
			err = errInvalidEvent(v, ev)
		}

		if err != nil {
			c.OnError(err)
		}
	})
}

func parsePeerUpdate(c *Client, v *fastjson.Value) (p PeerUpdate, err error) {
	if err = jsonHex(v, p.AccountID[:], "public_key"); err != nil {
		return
	}

	if err = jsonTime(v, &p.Time, "time"); err != nil {
		return
	}

	p.Address = string(v.GetStringBytes("address"))
	p.Message = string(v.GetStringBytes("message"))

	return p, nil
}

func parsePeerJoin(c *Client, v *fastjson.Value) error {
	u, err := parsePeerUpdate(c, v)
	if err != nil {
		return err
	}

	c.OnPeerJoin(PeerJoin{u})
	return nil
}

func parsePeerLeave(c *Client, v *fastjson.Value) error {
	u, err := parsePeerUpdate(c, v)
	if err != nil {
		return err
	}

	c.OnPeerLeave(PeerLeave{u})
	return nil
}
