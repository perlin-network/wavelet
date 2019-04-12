package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"io"
	"sort"
)

var (
	keyAccounts       = [...]byte{0x1}
	keyAccountBalance = [...]byte{0x2}
	keyAccountStake   = [...]byte{0x3}

	keyAccountContractCode     = [...]byte{0x4}
	keyAccountContractNumPages = [...]byte{0x5}
	keyAccountContractPages    = [...]byte{0x6}

	keyLedgerDifficulty = [...]byte{0x7}

	keyGraphRoot = [...]byte{0x8}

	keyCriticalTimestamps = [...]byte{0x9}
)

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

	logger := log.Account(id, "stake_updated")
	logger.Log().Uint64("stake", stake).Msg("Updated stake.")

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

	logger := log.Account(id, "num_pages_updated")
	logger.Log().Uint64("num_pages", numPages).Msg("Updated number of memory pages for a contract.")

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

func ReadCriticalTimestamps(kv store.KV) ([]uint64, error) {
	data, err := kv.Get(keyCriticalTimestamps[:])
	if err != nil {
		if err.Error() == "key not found" {
			return []uint64{}, nil
		}
		return nil, err
	}

	timestamps := make([]uint64, sys.CriticalTimestampAverageWindowSize)
	if err = binary.Read(bytes.NewReader(data), binary.LittleEndian, timestamps); err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return []uint64{}, nil
		}

		return nil, err
	}

	var actualTs []uint64
	for _, ts := range timestamps {
		if ts != 0 {
			actualTs = append(actualTs, ts)
		}
	}

	return actualTs, nil
}

func WriteCriticalTimestamp(kv store.KV, timestamp uint64) error {
	addTimestamp := func(ts uint64, timestamps []uint64) []uint64 {
		if len(timestamps) == 0 {
			return append(timestamps, ts)
		}

		toInsert := sort.Search(
			len(timestamps),
			func(i int) bool { return timestamps[i] >= ts },
		)

		if toInsert == len(timestamps) {
			return append(timestamps, ts)
		}

		return append(
			timestamps[:toInsert],
			append([]uint64{ts}, timestamps[toInsert:]...)...,
		)
	}

	timestamps, err := ReadCriticalTimestamps(kv)
	if err != nil {
		return err
	}

	newTimestamps := addTimestamp(timestamp, timestamps)
	newTimestampsSize := len(newTimestamps)

	if newTimestampsSize < sys.CriticalTimestampAverageWindowSize {
		for i := 0; i < sys.CriticalTimestampAverageWindowSize-newTimestampsSize; i++ {
			newTimestamps = append(newTimestamps, 0)
		}
	} else if newTimestampsSize > sys.CriticalTimestampAverageWindowSize {
		newTimestamps = newTimestamps[len(newTimestamps)-sys.CriticalTimestampAverageWindowSize:]
	}

	buf := bytes.NewBuffer(nil)
	if err := binary.Write(buf, binary.LittleEndian, newTimestamps); err != nil {
		return err
	}

	return kv.Put(keyCriticalTimestamps[:], buf.Bytes())
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
