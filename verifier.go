package wavelet

import (
	"bytes"
	"context"
	"errors"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"sync"
)

type verificationRequest struct {
	tx *Transaction
	response  chan error
}

type verifier struct {
	wg     sync.WaitGroup
	cancel func()
	bus    chan verificationRequest
}

func validateTx(tx *Transaction) error {
	if tx.ID == common.ZeroTransactionID {
		return errors.New("tx must have an id")
	}

	if tx.Sender == common.ZeroAccountID {
		return errors.New("tx must have sender associated to it")
	}

	if tx.Creator == common.ZeroAccountID {
		return errors.New("tx must have a creator associated to it")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.New("transaction has no parents")
	}

	// Check that parents are lexicographically sorted, are not itself, and are unique.
	set := make(map[common.TransactionID]struct{})

	for i := len(tx.ParentIDs) - 1; i > 0; i-- {
		if tx.ID == tx.ParentIDs[i] {
			return errors.New("tx must not include itself in its parents")
		}

		if bytes.Compare(tx.ParentIDs[i-1][:], tx.ParentIDs[i][:]) > 0 {
			return errors.New("tx must have sorted parent ids")
		}

		if _, duplicate := set[tx.ParentIDs[i]]; duplicate {
			return errors.New("tx must not have duplicate parent ids")
		}

		set[tx.ParentIDs[i]] = struct{}{}
	}

	if tx.Tag > sys.TagStake {
		return errors.New("tx has an unknown tag")
	}

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.New("tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.New("tx must have no payload if is a nop transaction")
	}

	var nonce [8]byte // TODO(kenta): nonce

	if !edwards25519.Verify(tx.Creator, append(nonce[:], append([]byte{tx.Tag}, tx.Payload...)...), tx.CreatorSignature) {
		return errors.New("tx has invalid creator signature")
	}

	cpy := *tx
	cpy.SenderSignature = common.ZeroSignature

	if !edwards25519.Verify(tx.Sender, cpy.Marshal(), tx.SenderSignature) {
		return errors.New("tx has invalid sender signature")
	}

	return nil
}

func NewVerifier(workersNum int, capacity uint32) *verifier {
	ctx, cancel := context.WithCancel(context.Background())
	v := verifier{
		cancel: cancel,
		bus:    make(chan verificationRequest, capacity),
	}

	v.wg.Add(workersNum)
	for i := 0; i < workersNum; i++ {
		go func(ctx context.Context) {
			defer v.wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case request := <-v.bus:
					request.response <- validateTx(request.tx)
				}
			}
		}(ctx)
	}

	return &v
}

func (v *verifier) Stop() {
	v.cancel()
	v.wg.Wait()
	close(v.bus)
}

func (v *verifier) verify(tx *Transaction) error {
	request := verificationRequest{
		tx: tx,
		response:  make(chan error, 1),
	}
	v.bus <- request
	return <-request.response
}
