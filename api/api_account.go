package api

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type Account struct {
	Balance    uint64 `json:"balance"`
	GasBalance uint64 `json:"gas_balance"`
	Stake      uint64 `json:"stake"`
	Reward     uint64 `json:"reward"`
	Nonce      uint64 `json:"nonce"`
	IsContract bool   `json:"is_contract"`
	NumPages   uint64 `json:"num_pages"`

	// Internal fields.
	id     wavelet.AccountID
	ledger *wavelet.Ledger
}

var _ MarshalableJSON = (*Account)(nil)

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
	nonce, _ := wavelet.ReadAccountNonce(snapshot, id)
	_, isContract := wavelet.ReadAccountContractCode(snapshot, id)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, id)

	g.render(ctx, &Account{
		ledger:     g.Ledger,
		id:         id,
		Balance:    balance,
		GasBalance: gasBalance,
		Stake:      stake,
		Reward:     reward,
		Nonce:      nonce,
		IsContract: isContract,
		NumPages:   numPages,
	})
}

func (s *Account) MarshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.id == wavelet.ZeroAccountID {
		return nil, errors.New("insufficient fields specified")
	}

	o := arena.NewObject()

	arenaSet(arena, o, "public_key", s.id[:])
	arenaSet(arena, o, "balance", s.Balance)
	arenaSet(arena, o, "gas_balance", s.GasBalance)
	arenaSet(arena, o, "stake", s.Stake)
	arenaSet(arena, o, "reward", s.Reward)
	arenaSet(arena, o, "nonce", s.Nonce)
	arenaSet(arena, o, "is_contract", s.IsContract)

	if s.NumPages != 0 {
		arenaSet(arena, o, "num_pages", s.NumPages)
	}

	return o.MarshalTo(nil), nil
}
