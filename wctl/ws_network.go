package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollNetwork() (func(), error) {
	return c.pollWS(RouteWSNetwork, func(v *fastjson.Value) {
		var err error

		if err := checkMod(v, "network"); err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
			return
		}

		switch ev := jsonString(v, "event"); ev {
		case "joined":
			err = parsePeerJoin(c, v)
		case "left":
			err = parsePeerLeave(c, v)
		default:
			// err = errInvalidEvent(v, ev)
		}

		if err != nil {
			if c.OnError != nil {
				c.OnError(err)
			}
		}
	})
}

func parsePeerUpdate(v *fastjson.Value) (p PeerUpdate, err error) {
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
	u, err := parsePeerUpdate(v)
	if err != nil {
		return err
	}

	if c.OnPeerJoin != nil {
		c.OnPeerJoin(PeerJoin{u})
	}

	return nil
}

func parsePeerLeave(c *Client, v *fastjson.Value) error {
	u, err := parsePeerUpdate(v)
	if err != nil {
		return err
	}

	if c.OnPeerLeave != nil {
		c.OnPeerLeave(PeerLeave{u})
	}

	return nil
}
