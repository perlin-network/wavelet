package wavelet

import (
	"github.com/dghubble/trie"
	"io"
	"strings"
	"sync"
)

// Indexer indexes all transaction IDs into a single trie for the
// purposes of suiting the needs of implementing autocomplete
// related components.
type Indexer struct {
	sync.RWMutex
	index *trie.PathTrie
}

// NewIndexer instantiates trie indices for indexing complete
// transactions by their ID.
func NewIndexer() *Indexer {
	return &Indexer{index: trie.NewPathTrie()}
}

// Index indexes a single hex-encoded transaction ID. This
// method is safe to call concurrently.
func (m *Indexer) Index(id string) {
	m.Lock()
	m.index.Put(id, struct{}{})
	m.Unlock()
}

// Remove un-indexes a single hex-encoded transaction ID. This
// method is safe to call concurrently.
func (m *Indexer) Remove(id string) {
	m.Lock()
	m.index.Delete(id)
	m.Unlock()
}

// Find searches through complete transaction indices for a specified
// query string. All indices that queried are in the form of tries.
func (m *Indexer) Find(query string, count int) []string {
	results := make([]string, 0, count)

	m.RLock()
	defer m.RUnlock()

	_ = m.index.Walk(func(key string, _ interface{}) error {
		if len(results) >= count {
			return io.EOF
		}

		if !strings.HasPrefix(key, query) {
			return io.EOF
		}

		results = append(results, key)

		return nil
	})

	return results
}
