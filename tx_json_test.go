package wavelet

import (
	"testing"
	"testing/quick"
)

// TestParseJSON tests the functionality of the ParseJSON helper method.
func TestParseJSON(t *testing.T) {
	f := func(jsonData []byte, tag string) bool {
		payload, err := ParseJSON(jsonData, tag) // Attempt to parse
		return !(err != nil && payload != nil)   // Check errored but still returned
	}

	if err := quick.Check(f, nil); err != nil { // Check for errors
		t.Fatal(err) // Panic
	}
}
