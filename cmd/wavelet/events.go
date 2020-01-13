package main

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/rs/zerolog"
)

// converts a normal (func to close, error) to only an error
func addToCloser(closer *[]func(), fns ...func() (func(), error)) error {
	for _, fn := range fns {
		close, err := fn()
		if err != nil {
			return err
		}

		*closer = append(*closer, close)
	}

	return nil
}

func setEvents(c *wctl.Client) (func(), error) {
	// api/events.go

	var toClose = []func(){}
	var cleanup = func() {
		for _, f := range toClose {
			f()
		}
	}

	c.OnError = func(err error) {
		log.ErrNode(err, "WS Error occured.")
	}

	toClose = append(toClose, c.AddHandler(func(ev log.MarshalableEvent) {
		switch ev.(type) {
		case *wavelet.Metrics, *wavelet.TxApplied:
			// too verbose, ignore
			return
		}

		ev.MarshalEvent(log.Node().Info())
	}))

	err := addToCloser(
		&toClose,
		c.PollNetwork,
		c.PollAccounts,
		c.PollContracts,
		c.PollTransactions,
		c.PollConsensus,
	)

	return cleanup, err
}

func withCustomMessage(ev *zerolog.Event, v log.MarshalableEvent, msg string) {
	dict := zerolog.Dict()
	v.MarshalEvent(dict)
	ev.Dict("tx", dict)
	ev.Msg(msg)
}
