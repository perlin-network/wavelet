package wavelet

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"testing/quick"
)

// TestParseJSON tests the functionality of the ParseJSON helper method.
func TestParseJSON(t *testing.T) {
	t.Parallel()

	f := func(jsonData []byte, tag string) bool {
		payload, err := ParseJSON(jsonData, tag) // Attempt to parse
		return !(err != nil && payload != nil)   // Check errored but still returned
	}

	assert.NoError(t, quick.Check(f, nil)) // Check no errors
}
