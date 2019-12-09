// +build unit

package wavelet

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockUnmarshal(t *testing.T) {
	var nodeID MerkleNodeID
	_, err := rand.Read(nodeID[:])
	assert.NoError(t, err)

	txIds := make([]TransactionID, 0, 3)

	for i := 0; i < cap(txIds); i++ {
		var id TransactionID
		_, err := rand.Read(id[:])
		assert.NoError(t, err)

		txIds = append(txIds, id)
	}

	original := NewBlock(10, nodeID, txIds...)

	after, err := UnmarshalBlock(bytes.NewReader(original.Marshal()))
	assert.NoError(t, err)

	assert.EqualValues(t, original, after)
}
