// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"github.com/armon/go-radix"
)

// Indexer indexes all transaction IDs into a single trie for the
// purposes of suiting the needs of implementing autocomplete
// related components.
type Indexer struct {
	*radix.Tree
}

// NewIndexer instantiates trie indices for indexing complete
// transactions by their ID.
func NewIndexer() *Indexer {
	return &Indexer{
		radix.New(),
	}
}

// Index indexes a single hex-encoded transaction ID. This
// method is safe to call concurrently.
func (m *Indexer) Index(id string) {
	m.Insert(id, nil)
}

// Remove un-indexes a single hex-encoded transaction ID. This
// method is safe to call concurrently.
func (m *Indexer) Remove(id string) {
	m.Delete(id)
}

// Find searches through complete transaction indices for a specified
// query string. All indices that queried are in the form of tries.
func (m *Indexer) Find(query string, max int) (results []string) {
	if max > 0 {
		results = make([]string, 0, max)
	}

	m.WalkPrefix(query, func(a string, _ interface{}) bool {
		if max > 0 && len(results) >= max {
			return false
		}

		results = append(results, a)
		return true
	})

	return results
}
