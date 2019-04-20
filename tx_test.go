package wavelet

import (
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

func TestTransaction_ExpectedDifficulty(t *testing.T) {
	const MinDifficulty = uint64(8)

	assert.Equal(t, uint64(9), Transaction{Depth: 54638, Confidence: 196398}.ExpectedDifficulty(MinDifficulty))

	// Genesis transaction.
	assert.Equal(t, MinDifficulty, Transaction{Depth: 0, Confidence: 0}.ExpectedDifficulty(MinDifficulty))

	// Test upper bounds.
	assert.Equal(t, uint64(34), Transaction{Depth: 19638, Confidence: math.MaxUint64}.ExpectedDifficulty(MinDifficulty))
	assert.Equal(t, MinDifficulty, Transaction{Depth: math.MaxUint64, Confidence: 19638}.ExpectedDifficulty(MinDifficulty))
}
