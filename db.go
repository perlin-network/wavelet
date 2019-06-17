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

package wavelet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"io"
	"strconv"
)

var (
	keyAccounts       = [...]byte{0x1}
	keyAccountNonce   = [...]byte{0x2}
	keyAccountBalance = [...]byte{0x3}
	keyAccountStake   = [...]byte{0x4}

	keyAccountContractCode     = [...]byte{0x5}
	keyAccountContractNumPages = [...]byte{0x6}
	keyAccountContractPages    = [...]byte{0x7}

	keyRounds           = [...]byte{0x8}
	keyRoundLatestIx    = [...]byte{0x9}
	keyRoundOldestIx    = [...]byte{0x10}
	keyRoundStoredCount = [...]byte{0x11}

	keyAccountReward = [...]byte{0x12}
	keyRewardWithdrawals = [...]byte{0x13}
)

type RewardWithdrawal struct {
	accountID AccountID
	amount uint64
	round uint64
}

func (rw RewardWithdrawal) Key() []byte {
	return append(
		keyRewardWithdrawals[:],
		append(
			[]byte(strconv.FormatUint(rw.round, 10)),
			rw.accountID[:]...,
		)...,
	)
}

func (rw RewardWithdrawal) Marshal() []byte {
	var w bytes.Buffer

	w.Write(rw.accountID[:])

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], rw.amount)
	w.Write(buf[:8])

	binary.BigEndian.PutUint64(buf[:], rw.round)
	w.Write(buf[:8])

	return w.Bytes()
}

func UnmarshalRewardWithdrawal(r io.Reader) (RewardWithdrawal, error) {
	var rw RewardWithdrawal
	if _, err := io.ReadFull(r, rw.accountID[:]); err != nil {
		err = errors.Wrap(err, "failed to decode reward withdrawal account id")
		return rw, err
	}

	var buf [8]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "failed to decode reward withdrawal amount")
		return rw, err
	}

	rw.amount = binary.BigEndian.Uint64(buf[:8])

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "failed to decode reward withdrawal round")
		return rw, err
	}

	rw.round = binary.BigEndian.Uint64(buf[:8])

	return rw, nil
}

func ReadAccountNonce(tree *avl.Tree, id AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountNonce[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountNonce(tree *avl.Tree, id AccountID, nonce uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], nonce)

	writeUnderAccounts(tree, id, keyAccountNonce[:], buf[:])
}

func ReadAccountBalance(tree *avl.Tree, id AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountBalance(tree *avl.Tree, id AccountID, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	writeUnderAccounts(tree, id, keyAccountBalance[:], buf[:])
}

func ReadAccountStake(tree *avl.Tree, id AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountStake(tree *avl.Tree, id AccountID, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	writeUnderAccounts(tree, id, keyAccountStake[:], buf[:])
}

func ReadAccountReward(tree *avl.Tree, id AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountReward[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountReward(tree *avl.Tree, id AccountID, reward uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], reward)
	writeUnderAccounts(tree, id, keyAccountReward[:], buf[:])
}

func ReadAccountContractCode(tree *avl.Tree, id TransactionID) ([]byte, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractCode[:])
	if !exists || len(buf) == 0 {
		return nil, false
	}

	return buf, true
}

func WriteAccountContractCode(tree *avl.Tree, id TransactionID, code []byte) {
	writeUnderAccounts(tree, id, keyAccountContractCode[:], code[:])
}

func ReadAccountContractNumPages(tree *avl.Tree, id TransactionID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractNumPages[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountContractNumPages(tree *avl.Tree, id TransactionID, numPages uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], numPages)

	writeUnderAccounts(tree, id, keyAccountContractNumPages[:], buf[:])
}

func ReadAccountContractPage(tree *avl.Tree, id TransactionID, idx uint64) ([]byte, bool) {
	var idxBuf [8]byte
	binary.LittleEndian.PutUint64(idxBuf[:], idx)

	buf, exists := readUnderAccounts(tree, id, append(keyAccountContractPages[:], idxBuf[:]...))
	if !exists || len(buf) == 0 {
		return nil, false
	}

	decoded, err := snappy.Decode(nil, buf)
	if err != nil {
		return nil, false
	}

	return decoded, true
}

func WriteAccountContractPage(tree *avl.Tree, id TransactionID, idx uint64, page []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	encoded := snappy.Encode(nil, page)

	writeUnderAccounts(tree, id, append(keyAccountContractPages[:], buf[:]...), encoded)
}

func readUnderAccounts(tree *avl.Tree, id AccountID, key []byte) ([]byte, bool) {
	buf, exists := tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func writeUnderAccounts(tree *avl.Tree, id AccountID, key, value []byte) {
	tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}

func StoreRound(
	kv store.KV, round Round, currentIx, oldestIx uint32, storedCount uint8,
) error {
	if err := kv.Put(keyRoundStoredCount[:], []byte{byte(storedCount)}); err != nil {
		return errors.Wrap(err, "error storing stored rounds count")
	}

	var oldestIxBuf [4]byte
	binary.BigEndian.PutUint32(oldestIxBuf[:], oldestIx)
	if err := kv.Put(keyRoundOldestIx[:], oldestIxBuf[:]); err != nil {
		return errors.Wrap(err, "error storing oldest round index")
	}

	var currentIxBuf [4]byte
	binary.BigEndian.PutUint32(currentIxBuf[:], currentIx)
	if err := kv.Put(keyRoundLatestIx[:], currentIxBuf[:]); err != nil {
		return errors.Wrap(err, "error storing latest round index")
	}

	if err := kv.Put(append(keyRounds[:], strconv.Itoa(int(currentIx))...), round.Marshal()); err != nil {
		return errors.Wrap(err, "error storing round")
	}

	return nil
}

func LoadRounds(kv store.KV) ([]*Round, uint32, uint32, error) {
	var b []byte
	var err error

	b, err = kv.Get(keyRoundLatestIx[:])
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "error loading latest round index")
	}
	latestIx := binary.BigEndian.Uint32(b[:4])

	b, err = kv.Get(keyRoundOldestIx[:])
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "error loading oldest round index")
	}
	oldestIx := binary.BigEndian.Uint32(b[:4])

	b, err = kv.Get(keyRoundStoredCount[:])
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "error loading oldest round index")
	}
	storedCount := int(b[0])

	rounds := make([]*Round, storedCount)
	for i := 0; i < storedCount; i++ {
		b, err = kv.Get(append(keyRounds[:], strconv.Itoa(i)...))
		if err != nil {
			return nil, 0, 0, errors.Wrap(err, fmt.Sprintf("error loading round - %d", i))
		}

		round, err := UnmarshalRound(bytes.NewReader(b))
		if err != nil {
			return nil, 0, 0, errors.Wrap(err, "error unmarshaling round")
		}

		rounds[i] = &round
	}

	return rounds, latestIx, oldestIx, nil
}

func GetRewardWithdrawals(kv store.KV, roundLimit uint64) ([]RewardWithdrawal, error) {
	filter := func(k, v []byte) bool {
		rw, err := UnmarshalRewardWithdrawal(bytes.NewReader(v))
		if err != nil {
			return false
		}

		return rw.round <= roundLimit
	}

	data, err := kv.GetRange(keyRewardWithdrawals[:], filter)
	if err != nil {
		return nil, err
	}

	rws := make([]RewardWithdrawal, len(data))
	for i, d := range data {
		rw, err := UnmarshalRewardWithdrawal(bytes.NewReader(d))
		if err != nil {
			return nil, err
		}

		rws[i] = rw
	}

	return rws, nil
}

func StoreRewardWithdrawal(kv store.KV, rw RewardWithdrawal) error {
	return kv.Put(rw.Key(), rw.Marshal())
}
