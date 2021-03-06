package wctl

import "github.com/valyala/fastjson"

func (c *Client) PollMetrics() (func(), error) {
	return c.pollWS(RouteWSMetrics, func(v *fastjson.Value) {
		met := Metrics{
			BlocksQueried:      v.GetUint64("blocks.queried"),
			BlocksFinalized:    v.GetFloat64("blocks.finalized"),
			TxGossiped:         v.GetUint64("tx.gossiped"),
			TxReceived:         v.GetUint64("tx.received"),
			TxAccepted:         v.GetUint64("tx.accepted"),
			TxDownloaded:       v.GetUint64("tx.downloaded"),
			BpsQueried:         v.GetFloat64("bps.queried"),
			TpsGossiped:        v.GetFloat64("tps.gossiped"),
			TpsReceived:        v.GetFloat64("tps.received"),
			TpsAccepted:        v.GetFloat64("tps.accepted"),
			TpsDownloaded:      v.GetFloat64("tps.downloaded"),
			QueryLatencyMaxMS:  v.GetInt64("query.latency.max.ms"),
			QueryLatencyMinMS:  v.GetInt64("query.latency.min.ms"),
			QueryLatencyMeanMS: v.GetFloat64("query.latency.mean.ms"),
			Message:            string(v.GetStringBytes("message")),
		}

		if err := jsonTime(v, &met.Time, "time"); err != nil {
			c.OnError(err)
			return
		}

		c.OnMetrics(met)
	})
}
