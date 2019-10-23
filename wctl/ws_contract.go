package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollContracts() (func(), error) {
	return c.pollWS(RouteWSContracts, func(v *fastjson.Value) {
		var err error

		for _, o := range v.GetArray() {
			if err := checkMod(o, "contract"); err != nil {
				if c.OnError != nil {
					c.OnError(err)
				}
				continue
			}

			switch ev := jsonString(o, "event"); ev {
			case "gas":
				err = parseContractGas(c, o)
			case "log":
				err = parseContractLog(c, o)
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

func parseContractGas(c *Client, v *fastjson.Value) error {
	var g ContractGas

	if err := jsonHex(v, g.SenderID[:], "sender_id"); err != nil {
		return err
	}

	if err := jsonHex(v, g.ContractID[:], "contract_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &g.Time, "time"); err != nil {
		return err
	}

	g.Gas = v.GetUint64("gas")
	g.GasLimit = v.GetUint64("gas_limit")
	g.Message = string(v.GetStringBytes("message"))

	if c.OnContractGas != nil {
		c.OnContractGas(g)
	}
	return nil
}

func parseContractLog(c *Client, v *fastjson.Value) error {
	var l ContractLog

	if err := jsonHex(v, l.ContractID[:], "contract_id"); err != nil {
		return err
	}

	if err := jsonTime(v, &l.Time, "time"); err != nil {
		return err
	}

	l.Message = string(v.GetStringBytes("message"))

	if c.OnContractLog != nil {
		c.OnContractLog(l)
	}
	return nil
}
