// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package log

import (
	"io"

	"github.com/rs/zerolog"
)

var (
	output = &multiWriter{
		writers: make(map[string]io.Writer),
	}
	logger = zerolog.New(output).With().Timestamp().Logger()

	node      zerolog.Logger
	network   zerolog.Logger
	accounts  zerolog.Logger
	consensus zerolog.Logger
	contract  zerolog.Logger
	syncer    zerolog.Logger
	stake     zerolog.Logger
	tx        zerolog.Logger
	metrics   zerolog.Logger
)

const (
	LoggerWavelet   = "wavelet"
	LoggerWebsocket = "ws"

	KeyModule = "mod"
	KeyEvent  = "event"

	ModuleNode      = "node"
	ModuleNetwork   = "network"
	ModuleAccounts  = "accounts"
	ModuleConsensus = "consensus"
	ModuleContract  = "contract"
	ModuleSync      = "sync"
	ModuleStake     = "stake"
	ModuleTX        = "tx"
	ModuleMetrics   = "metrics"
)

func init() {
	setupChildLoggers()
}

func setupChildLoggers() {
	node = logger.With().Str(KeyModule, ModuleNode).Logger()
	network = logger.With().Str(KeyModule, ModuleNetwork).Logger()
	accounts = logger.With().Str(KeyModule, ModuleAccounts).Logger()
	consensus = logger.With().Str(KeyModule, ModuleConsensus).Logger()
	contract = logger.With().Str(KeyModule, ModuleContract).Logger()
	syncer = logger.With().Str(KeyModule, ModuleSync).Logger()
	stake = logger.With().Str(KeyModule, ModuleStake).Logger()
	tx = logger.With().Str(KeyModule, ModuleTX).Logger()
	metrics = logger.With().Str(KeyModule, ModuleMetrics).Logger()
}

func SetLevel(level string) {
	if l, err := zerolog.ParseLevel(level); err == nil {
		node = node.Level(l)
		network = network.Level(l)
		accounts = accounts.Level(l)
		consensus = consensus.Level(l)
		contract = contract.Level(l)
		syncer = syncer.Level(l)
		stake = stake.Level(l)
		tx = tx.Level(l)
		metrics = metrics.Level(l)
	}
}

func SetWriter(key string, writer io.Writer) {
	output.SetWriter(key, writer)
}

func Node() zerolog.Logger {
	return node
}

func Network(event string) zerolog.Logger {
	return network.With().Str(KeyEvent, event).Logger()
}

func Accounts(event string) zerolog.Logger {
	return accounts.With().Str(KeyEvent, event).Logger()
}

func Contracts(event string) zerolog.Logger {
	return contract.With().Str(KeyEvent, event).Logger()
}

func TX(event string) zerolog.Logger {
	return tx.With().Str(KeyEvent, event).Logger()
}

func Consensus(event string) zerolog.Logger {
	return consensus.With().Str(KeyEvent, event).Logger()
}

func Stake(event string) zerolog.Logger {
	return stake.With().Str(KeyEvent, event).Logger()
}

func Sync(event string) zerolog.Logger {
	return syncer.With().Str(KeyEvent, event).Logger()
}

func Metrics() zerolog.Logger {
	return metrics
}
