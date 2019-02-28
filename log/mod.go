package log

import (
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"io"
	"os"
)

var (
	output = new(multiWriter)
	logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(output)

	node        *zerolog.Logger
	accounts    *zerolog.Logger
	broadcaster *zerolog.Logger
	consensus   *zerolog.Logger
	contract    *zerolog.Logger
	stake       *zerolog.Logger
	tx          *zerolog.Logger
)

const (
	KeyModule = "mod"
	KeyEvent  = "event"

	ModuleNode        = "node"
	ModuleAccounts    = "accounts"
	ModuleBroadcaster = "broadcaster"
	ModuleConsensus   = "consensus"
	ModuleContract    = "contract"
	ModuleStake       = "stake"
	ModuleTx          = "tx"
)

func Register(w io.Writer) {
	if w != nil {
		output.Register(w)
	}
}

func init() {
	Register(NewConsoleWriter())

	setupChildLoggers()
}

func setupChildLoggers() {
	a := logger.With().Str(KeyModule, ModuleNode).Logger()
	node = &a

	b := logger.With().Str(KeyModule, ModuleAccounts).Logger()
	accounts = &b

	c := logger.With().Str(KeyModule, ModuleBroadcaster).Logger()
	broadcaster = &c

	d := logger.With().Str(KeyModule, ModuleConsensus).Logger()
	consensus = &d

	e := logger.With().Str(KeyModule, ModuleContract).Logger()
	contract = &e

	f := logger.With().Str(KeyModule, ModuleStake).Logger()
	stake = &f

	g := logger.With().Str(KeyModule, ModuleTx).Logger()
	tx = &g
}

func Node() *zerolog.Logger {
	return node
}

func Account(id [sys.PublicKeySize]byte, event string) *zerolog.Logger {
	accounts := accounts.With().Hex("account_id", id[:]).Str(KeyEvent, event).Logger()
	return &accounts
}

func Contracts() *zerolog.Logger {
	return contract
}

func Contract(id [sys.TransactionIDSize]byte, event string) *zerolog.Logger {
	contract := contract.With().Hex("contract_id", id[:]).Str(KeyEvent, event).Logger()
	return &contract
}

func Broadcaster() *zerolog.Logger {
	return broadcaster
}

func TX(id [sys.TransactionIDSize]byte, event string) *zerolog.Logger {
	tx := tx.With().Hex("tx_id", id[:]).Str(KeyEvent, event).Logger()
	return &tx
}

func Consensus(event string) *zerolog.Logger {
	consensus := consensus.With().Str(KeyEvent, event).Logger()
	return &consensus
}

func Stake(event string) *zerolog.Logger {
	stake := stake.With().Str(KeyEvent, event).Logger()
	return &stake
}
