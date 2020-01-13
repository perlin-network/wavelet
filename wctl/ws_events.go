package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

// EventsValue could be used on all WS routes, and it would be safe to do so;
// this is because all event strings are different.
var EventsValue = map[string]func() log.Loggable{
	// Accounts
	"balance_updated":     func() log.Loggable { return &wavelet.AccountBalanceUpdated{} },
	"gas_balance_updated": func() log.Loggable { return &wavelet.AccountGasBalanceUpdated{} },
	"num_pages_updated":   func() log.Loggable { return &wavelet.AccountNumPagesUpdated{} },
	"stake_updated":       func() log.Loggable { return &wavelet.AccountStakeUpdated{} },
	"reward_updated":      func() log.Loggable { return &wavelet.AccountRewardUpdated{} },

	// Consensus
	"finalized": func() log.Loggable { return &wavelet.ConsensusFinalized{} },
	"proposal":  func() log.Loggable { return &wavelet.ConsensusProposal{} },

	// Contract
	"execute": func() log.Loggable { return &wavelet.ContractGas{} },

	// Metrics
	"metrics": func() log.Loggable { return &wavelet.Metrics{} },

	// Network
	"joined": func() log.Loggable { return &api.NetworkJoined{} },
	"left":   func() log.Loggable { return &api.NetworkLeft{} },

	// Transactions
	"applied":  func() log.Loggable { return &wavelet.TxApplied{} },
	"rejected": func() log.Loggable { return &wavelet.TxRejected{} },
}

func (c *Client) genericHandler(v *fastjson.Value) error {
	event := log.ValueString(v, "event")
	fn, ok := EventsValue[event]
	if !ok {
		return errInvalidEvent(v, event)
	}

	var value = fn()

	if err := value.UnmarshalValue(v); err != nil {
		return err
	}

	for _, h := range c.handlers {
		h(value)
	}

	return nil
}

// PollTransactions calls the callback for each WS event received. On error, the
// callback may be called twice.
func (c *Client) PollTransactions() (func(), error) {
	return c.pollWS(RouteWSTransactions, c.genericHandler)
}

func (c *Client) PollNetwork() (func(), error) {
	return c.pollWS(RouteWSNetwork, c.genericHandler)
}

func (c *Client) PollMetrics() (func(), error) {
	return c.pollWS(RouteWSMetrics, c.genericHandler)
}

func (c *Client) PollContracts() (func(), error) {
	return c.pollWSArray(RouteWSContracts, c.genericHandler)
}

func (c *Client) PollConsensus() (func(), error) {
	return c.pollWS(RouteWSConsensus, c.genericHandler)
}

func (c *Client) PollAccounts() (func(), error) {
	return c.pollWSArray(RouteWSAccounts, c.genericHandler)
}
