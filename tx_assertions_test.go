package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAssertInView(t *testing.T) {
	kv := store.NewInmem()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	t.Run("critical, wrong view id", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		assert.EqualError(
			t,
			AssertInView(1, kv, tx, true),
			"critical transaction was made for view ID 0, but our view ID is 1",
		)
	})

	t.Run("critical, empty merkle root", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, true),
			"critical transactions merkle root is expected to be not nil",
		)
	})

	t.Run("critical, not enough critical timestamps", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		tx.AccountsMerkleRoot = [16]byte{0, 1, 2, 3}
		tx.ViewID = 2

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, true),
			"expected tx to have 2 timestamp(s), but has 0 timestamp(s)",
		)
	})

	t.Run("critical, critical timestamps in wrong order", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		tx.AccountsMerkleRoot = [16]byte{0, 1, 2, 3}
		tx.ViewID = 3
		tx.DifficultyTimestamps = []uint64{2, 1, 3}

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, true),
			"tx critical timestamps are not in ascending order",
		)
	})

	t.Run("critical, critical timestamps differ from stored", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		if !assert.NoError(t, WriteCriticalTimestamp(kv, 4)) {
			return
		}

		tx.AccountsMerkleRoot = [16]byte{0, 1, 2, 3}
		tx.ViewID = 3
		tx.DifficultyTimestamps = []uint64{1, 2, 3}

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, true),
			"tx critical timestamps differ from the stored ones",
		)
	})

	t.Run("critical, success", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		if !assert.NoError(t, WriteCriticalTimestamp(kv, 4)) {
			return
		}

		tx.AccountsMerkleRoot = [16]byte{0, 1, 2, 3}
		tx.ViewID = 1
		tx.DifficultyTimestamps = []uint64{4}

		assert.NoError(t, AssertInView(tx.ViewID, kv, tx, true))
	})

	t.Run("non critical, not empty merkle root", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		tx.AccountsMerkleRoot = [16]byte{0, 1, 2, 3}

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, false),
			"transactions merkle root is expected to be nil",
		)
	})

	t.Run("non critical, not empty critical timestamps", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		tx.DifficultyTimestamps = []uint64{1}

		assert.EqualError(
			t,
			AssertInView(tx.ViewID, kv, tx, false),
			"normal transactions are not expected to have difficulty timestamps",
		)
	})

	t.Run("non critical, older view id", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		assert.EqualError(
			t,
			AssertInView(1, kv, tx, false),
			"transaction was made for view ID 0, but our view ID is 1",
		)
	})

	t.Run("non critical, success", func(t *testing.T) {
		tx, err := NewTransaction(keys, sys.TagTransfer, []byte("lorem ipsum"))
		if !assert.NoError(t, err) {
			return
		}

		assert.NoError(t, AssertInView(tx.ViewID, kv, tx, false))
	})
}
