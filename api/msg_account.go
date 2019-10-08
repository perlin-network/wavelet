package api

import (
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
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
