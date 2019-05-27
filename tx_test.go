package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func BenchmarkNewTX(b *testing.B) {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	}
}

func BenchmarkMarshalUnmarshalTX(b *testing.B) {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := UnmarshalTransaction(bytes.NewReader(tx.Marshal()))
		assert.NoError(b, err)
	}
}
