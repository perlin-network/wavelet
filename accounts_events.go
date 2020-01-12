package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

// Balance Update

// balance_updated
type AccountBalanceUpdated struct {
	AccountID AccountID `json:"account_id"`
	Balance   uint64    `json:"balance"`
}

var _ log.Loggable = (*AccountBalanceUpdated)(nil)

func (a AccountBalanceUpdated) MarshalEvent(ev *zerolog.Event) {
	marshalAccountIDandUint64(ev, a.AccountID, "balance", a.Balance, "balance")
}

func (a *AccountBalanceUpdated) UnmarshalValue(v *fastjson.Value) error {
	return unmarshalAccountIDandUint64(v, a.AccountID, "balance", &a.Balance)
}

// Gas Balance update

// gas_balance_updated
type AccountGasBalanceUpdated struct {
	AccountID  AccountID `json:"account_id"`
	GasBalance uint64    `json:"gas_balance"`
}

var _ log.Loggable = (*AccountGasBalanceUpdated)(nil)

func (a AccountGasBalanceUpdated) MarshalEvent(ev *zerolog.Event) {
	marshalAccountIDandUint64(ev, a.AccountID, "gas_balance", a.GasBalance, "gas balance")
}

func (a *AccountGasBalanceUpdated) UnmarshalValue(v *fastjson.Value) error {
	return unmarshalAccountIDandUint64(v, a.AccountID, "gas_balance", &a.GasBalance)
}

// Num Pages update

// num_pages_updated
type AccountNumPagesUpdated struct {
	AccountID AccountID `json:"account_id"`
	NumPages  uint64    `json:"num_pages"`
}

var _ log.Loggable = (*AccountNumPagesUpdated)(nil)

func (a AccountNumPagesUpdated) MarshalEvent(ev *zerolog.Event) {
	marshalAccountIDandUint64(ev, a.AccountID, "num_pages", a.NumPages, "pages")
}

func (a *AccountNumPagesUpdated) UnmarshalValue(v *fastjson.Value) error {
	return unmarshalAccountIDandUint64(v, a.AccountID, "num_pages", &a.NumPages)
}

// Stake update

// stake_updated
type AccountStakeUpdated struct {
	AccountID AccountID `json:"account_id"`
	Stake     uint64    `json:"stake"`
}

var _ log.Loggable = (*AccountStakeUpdated)(nil)

func (a AccountStakeUpdated) MarshalEvent(ev *zerolog.Event) {
	marshalAccountIDandUint64(ev, a.AccountID, "stake", a.Stake, "stake")
}

func (a *AccountStakeUpdated) UnmarshalValue(v *fastjson.Value) error {
	return unmarshalAccountIDandUint64(v, a.AccountID, "stake", &a.Stake)
}

// Reward update

// reward_updated
type AccountRewardUpdated struct {
	AccountID AccountID `json:"account_id"`
	Reward    uint64    `json:"reward"`
}

var _ log.Loggable = (*AccountRewardUpdated)(nil)

func (a AccountRewardUpdated) MarshalEvent(ev *zerolog.Event) {
	marshalAccountIDandUint64(ev, a.AccountID, "reward", a.Reward, "reward")
}

func (a *AccountRewardUpdated) UnmarshalValue(v *fastjson.Value) error {
	return unmarshalAccountIDandUint64(v, a.AccountID, "reward", &a.Reward)
}

func marshalAccountIDandUint64(ev *zerolog.Event, id AccountID, ukey string, u uint64, updated string) {
	ev.Hex("account_id", id[:])
	ev.Uint64(ukey, u)
	ev.Msg("Updated account " + updated + ".")
}

// yes, I know this is a terrible function name, it can't be helped
func unmarshalAccountIDandUint64(v *fastjson.Value, id AccountID, ukey string, u *uint64) error {
	if err := log.ValueHex(v, id[:], "account_id"); err != nil {
		return err
	}

	*u = v.GetUint64(ukey)
	return nil
}
