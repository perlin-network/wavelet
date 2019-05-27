package log

import (
	"github.com/rs/zerolog"
	"io"
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
	KeyModule = "mod"
	KeyEvent  = "event"

	ModuleNode      = "node"
	ModuleNetwork   = "network"
	ModuleAccounts  = "accounts"
	ModuleConsensus = "consensus"
	ModuleContract  = "contract"
	ModuleSync      = "sync"
	ModuleStake     = "stake"
	ModuleTx        = "tx"
	ModuleMetrics   = "metrics"
)

func Set(key string, w io.Writer) {
	output.Set(key, w)
}

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
	tx = logger.With().Str(KeyModule, ModuleTx).Logger()
	metrics = logger.With().Str(KeyModule, ModuleMetrics).Logger()
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
