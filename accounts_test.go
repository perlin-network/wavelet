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
	"bytes"
	"testing"
	"testing/quick"

	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
)

func TestSmartContract(t *testing.T) {
	fn := func(id TransactionID, code [2 * 1024]byte) bool {
		accounts := NewAccounts(store.NewInmem())
		tree := accounts.Snapshot()

		returned, available := ReadAccountContractCode(tree, id)
		if returned != nil || available == true {
			return false
		}

		WriteAccountContractCode(tree, id, code[:])

		returned, available = ReadAccountContractCode(tree, id)
		if !bytes.Equal(code[:], returned) || available == false {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}
