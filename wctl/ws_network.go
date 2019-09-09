package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollNetwork() (func(), error) {
	return c.pollWS(RouteWSNetwork, func(v *fastjson.Value) {
		var err error

		for _, o := range v.GetArray() {
			if err := checkMod(o, "network"); err != nil {
				c.OnError(err)
				continue
			}

			switch ev := jsonString(o, "event"); ev {
			case "joined":
				err = parsePeerJoin(c, o)
			case "left":
				err = parsePeerLeave(c, o)
			default:
				err = errInvalidEvent(o, ev)
			}

			if err != nil {
				c.OnError(err)
			}
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
