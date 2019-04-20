package wavelet

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTransaction_ExpectedDifficulty(t *testing.T) {
	const MinDifficulty = 8

	tx := Transaction{
		Depth:      54638,
		Confidence: 196398,
	}

	assert.Equal(t, uint64(9), tx.ExpectedDifficulty(MinDifficulty))
}
