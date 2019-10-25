package api

import (
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type account struct {
	// Internal fields.
	id     wavelet.AccountID
	ledger *wavelet.Ledger

	balance    uint64
	gasBalance uint64
	stake      uint64
	reward     uint64
	nonce      uint64
	isContract bool
	numPages   uint64
}

var _ marshalableJSON = (*account)(nil)

func (g *Gateway) getAccount(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", wavelet.SizeAccountID)))
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

	g.render(ctx, &account{
		ledger:     g.Ledger,
		id:         id,
		balance:    balance,
		gasBalance: gasBalance,
		stake:      stake,
		reward:     reward,
		nonce:      nonce,
		isContract: isContract,
		numPages:   numPages,
	})
}

func (s *account) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.id == wavelet.ZeroAccountID {
		return nil, errors.New("insufficient fields specified")
	}

	o := arena.NewObject()

	o.Set("public_key", arena.NewString(hex.EncodeToString(s.id[:])))
	o.Set("balance", arena.NewNumberString(strconv.FormatUint(s.balance, 10)))
	o.Set("gas_balance", arena.NewNumberString(strconv.FormatUint(s.gasBalance, 10)))
	o.Set("stake", arena.NewNumberString(strconv.FormatUint(s.stake, 10)))
	o.Set("reward", arena.NewNumberString(strconv.FormatUint(s.reward, 10)))
	o.Set("nonce", arena.NewNumberString(strconv.FormatUint(s.nonce, 10)))

	if s.isContract {
		o.Set("is_contract", arena.NewTrue())
	} else {
		o.Set("is_contract", arena.NewFalse())
	}

	if s.numPages != 0 {
		o.Set("num_mem_pages", arena.NewNumberString(strconv.FormatUint(s.numPages, 10)))
	}

	return o.MarshalTo(nil), nil
}
