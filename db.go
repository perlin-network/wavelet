package wavelet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
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

	keyRoundTotalCount  = [...]byte{0x8}
	keyRoundLatestIx = [...]byte{0x9}
	keyRoundOldestIx = [...]byte{0x10}
	keyRoundStoredCount = [...]byte{0x11}

	keyRound = "round"
)

func ReadAccountNonce(tree *avl.Tree, id common.AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountNonce[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountNonce(tree *avl.Tree, id common.AccountID, nonce uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], nonce)

	writeUnderAccounts(tree, id, keyAccountNonce[:], buf[:])
}

func ReadAccountBalance(tree *avl.Tree, id common.AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountBalance(tree *avl.Tree, id common.AccountID, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	writeUnderAccounts(tree, id, keyAccountBalance[:], buf[:])
}

func ReadAccountStake(tree *avl.Tree, id common.AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountStake(tree *avl.Tree, id common.AccountID, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	writeUnderAccounts(tree, id, keyAccountStake[:], buf[:])
}

func ReadAccountContractCode(tree *avl.Tree, id common.TransactionID) ([]byte, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractCode[:])
	if !exists || len(buf) == 0 {
		return nil, false
	}

	return buf, true
}

func WriteAccountContractCode(tree *avl.Tree, id common.TransactionID, code []byte) {
	writeUnderAccounts(tree, id, keyAccountContractCode[:], code[:])
}

func ReadAccountContractNumPages(tree *avl.Tree, id common.TransactionID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractNumPages[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountContractNumPages(tree *avl.Tree, id common.TransactionID, numPages uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], numPages)

	writeUnderAccounts(tree, id, keyAccountContractNumPages[:], buf[:])
}

func ReadAccountContractPage(tree *avl.Tree, id common.TransactionID, idx uint64) ([]byte, bool) {
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

func WriteAccountContractPage(tree *avl.Tree, id common.TransactionID, idx uint64, page []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	encoded := snappy.Encode(nil, page)

	writeUnderAccounts(tree, id, append(keyAccountContractPages[:], buf[:]...), encoded)
}

func readUnderAccounts(tree *avl.Tree, id common.AccountID, key []byte) ([]byte, bool) {
	buf, exists := tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func writeUnderAccounts(tree *avl.Tree, id common.AccountID, key, value []byte) {
	tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}

func StoreRound(
	kv store.KV, totalCount uint64, round Round,
	currentIx, oldestIx uint32, storedCount uint8,
) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], totalCount)
	if err := kv.Put(keyRoundTotalCount[:], buf[:]); err != nil {
		return errors.Wrap(err, "error storing total rounds count")
	}

	if err := kv.Put(keyRoundStoredCount[:], []byte{byte(storedCount)}); err != nil {
		return errors.Wrap(err, "error storing stored rounds count")
	}

	var ixBuf [4]byte
	binary.BigEndian.PutUint32(ixBuf[:], currentIx)
	if err := kv.Put(keyRoundLatestIx[:], ixBuf[:]); err != nil {
		return errors.Wrap(err, "error storing latest round index")
	}

	binary.BigEndian.PutUint32(ixBuf[:], oldestIx)
	if err := kv.Put(keyRoundOldestIx[:], ixBuf[:]); err != nil {
		return errors.Wrap(err, "error storing oldest round index")
	}

	if err := kv.Put([]byte(keyRound + strconv.Itoa(int(currentIx))), round.Marshal()); err != nil {
		return errors.Wrap(err, "error storing round")
	}

	return nil
}

func LoadRounds(kv store.KV) ([]*Round, uint64, uint32, uint32, error) {
	var b []byte
	var err error

	b, err = kv.Get(keyRoundTotalCount[:])
	if err != nil {
		return nil, 0, 0, 0, errors.Wrap(err, "error loading total rounds count")
	}
	totalCount := binary.BigEndian.Uint64(b[:8])

	b, err = kv.Get(keyRoundLatestIx[:])
	if err != nil {
		return nil, 0, 0, 0, errors.Wrap(err, "error loading latest round index")
	}
	fmt.Println(b)
	latestIx := binary.BigEndian.Uint32(b[:4])

	b, err = kv.Get(keyRoundOldestIx[:])
	if err != nil {
		return nil, 0, 0, 0, errors.Wrap(err, "error loading oldest round index")
	}
	oldestIx := binary.BigEndian.Uint32(b[:4])

	b, err = kv.Get(keyRoundStoredCount[:])
	if err != nil {
		return nil, 0, 0, 0, errors.Wrap(err, "error loading oldest round index")
	}
	storedCount := int(b[0])

	rounds := make([]*Round, storedCount)
	for i := 0; i < storedCount; i++ {
		b, err = kv.Get([]byte(keyRound + strconv.Itoa(i)))
		if err != nil {
			return nil, 0, 0, 0, errors.Wrap(err, fmt.Sprintf("error loading round - %d", i))
		}

		round, err := UnmarshalRound(bytes.NewReader(b))
		if err != nil {
			return nil, 0, 0, 0, errors.Wrap(err, "error unmarshaling round")
		}

		rounds[i] = &round
	}

	return rounds, totalCount, latestIx, oldestIx, nil
}
