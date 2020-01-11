package api

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type Account struct {
	PublicKey  wavelet.AccountID `json:"public_key"`
	Balance    uint64            `json:"balance"`
	GasBalance uint64            `json:"gas_balance"`
	Stake      uint64            `json:"stake"`
	Reward     uint64            `json:"reward"`
	IsContract bool              `json:"is_contract"`
	NumPages   uint64            `json:"num_mem_pages"`
}

var _ log.JSONObject = (*Account)(nil)

func (g *Gateway) getAccount(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(
			err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf(
			"account ID must be %d bytes long", wavelet.SizeAccountID)))
		return
	}

	var id wavelet.AccountID
	copy(id[:], slice)

	snapshot := g.Ledger.Snapshot()

	balance, _ := wavelet.ReadAccountBalance(snapshot, id)
	gasBalance, _ := wavelet.ReadAccountContractGasBalance(snapshot, id)
	stake, _ := wavelet.ReadAccountStake(snapshot, id)
	reward, _ := wavelet.ReadAccountReward(snapshot, id)
	_, isContract := wavelet.ReadAccountContractCode(snapshot, id)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, id)

	g.render(ctx, &Account{
		PublicKey:  id,
		Balance:    balance,
		GasBalance: gasBalance,
		Stake:      stake,
		Reward:     reward,
		IsContract: isContract,
		NumPages:   numPages,
	})
}

func (s *Account) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"public_key", s.PublicKey[:],
		"balance", s.Balance,
		"gas_balance", s.GasBalance,
		"stake", s.Stake,
		"reward", s.Reward,
		"is_contract", s.IsContract,
		"num_pages", s.NumPages)
}

func (s *Account) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"public_key", s.PublicKey[:],
		"balance", &s.Balance,
		"stake", &s.Stake,
		"reward", &s.Reward,
		"is_contract", &s.IsContract,
		"num_pages", &s.NumPages)
}

func (s *Account) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", s.PublicKey[:])
	ev.Uint64("balance", s.Balance)
	ev.Uint64("gas_balance", s.GasBalance)
	ev.Uint64("stake", s.Stake)
	ev.Uint64("reward", s.Reward)
	ev.Bool("is_contract", s.IsContract)
	ev.Uint64("num_pages", s.NumPages)

	ev.Msg("Account")
}
