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
		writers:        make(map[string]io.Writer),
		writersModules: make(map[string]map[string]struct{}),
	}
	logger = zerolog.New(output).With().Timestamp().Logger()

	node      zerolog.Logger
	network   zerolog.Logger
	accounts  zerolog.Logger
	consensus zerolog.Logger
	contract  zerolog.Logger
	syncer    zerolog.Logger
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
	ModuleTX        = "tx"
	ModuleMetrics   = "metrics"
)

func setupChildLoggers() {
	// Default to Debug
	node = logger.With().Str(KeyModule, ModuleNode).Logger().Level(zerolog.DebugLevel)
	network = logger.With().Str(KeyModule, ModuleNetwork).Logger().Level(zerolog.DebugLevel)
	accounts = logger.With().Str(KeyModule, ModuleAccounts).Logger().Level(zerolog.DebugLevel)
	consensus = logger.With().Str(KeyModule, ModuleConsensus).Logger().Level(zerolog.DebugLevel)
	contract = logger.With().Str(KeyModule, ModuleContract).Logger().Level(zerolog.DebugLevel)
	syncer = logger.With().Str(KeyModule, ModuleSync).Logger().Level(zerolog.DebugLevel)
	tx = logger.With().Str(KeyModule, ModuleTX).Logger().Level(zerolog.DebugLevel)
	metrics = logger.With().Str(KeyModule, ModuleMetrics).Logger().Level(zerolog.DebugLevel)
}

func SetLevel(ls string) {
	level, err := zerolog.ParseLevel(ls)
	if err != nil {
		level = zerolog.DebugLevel
	}

	node = node.Level(level)
	network = network.Level(level)
	accounts = accounts.Level(level)
	consensus = consensus.Level(level)
	contract = contract.Level(level)
	syncer = syncer.Level(level)
	tx = tx.Level(level)
	metrics = metrics.Level(level)
}

func SetWriter(key string, writer io.Writer) {
	var modules []string

	if cw, ok := writer.(ConsoleWriter); ok {
		for k := range cw.FilteredModules {
			modules = append(modules, k)
		}
	}

	output.SetWriter(key, writer, modules...)
}

func ClearWriter(key string) {
	output.Clear(key)
}

func Node() *zerolog.Logger {
	return &node
}

func Network(event string) *zerolog.Logger {
	return eventIf(event, network)
}

func Accounts(event string) *zerolog.Logger {
	return eventIf(event, accounts)
}

func Contracts(event string) *zerolog.Logger {
	return eventIf(event, contract)
}

func TX(event string) *zerolog.Logger {
	return eventIf(event, tx)
}

func Consensus(event string) *zerolog.Logger {
	return eventIf(event, consensus)
}

func Sync(event string) *zerolog.Logger {
	return eventIf(event, syncer)
}

func Metrics() *zerolog.Logger {
	return eventIf("", metrics)
}

func eventIf(event string, logger zerolog.Logger) *zerolog.Logger {
	var l zerolog.Logger
	if event == "" {
		l = logger.With().Logger()
	} else {
		l = logger.With().Str(KeyEvent, event).Logger()
	}

	return &l
}

// Write to only the writers that have filter for the module.
func Write(module string, msg []byte) error {
	_, err := output.WriteFilter(msg, module)
	return err
}
