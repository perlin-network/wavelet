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

// +build !integration,unit

package wavelet

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestCollapseResultsLogger(t *testing.T) {
	log.ClearWriters()
	defer log.ClearWriters()

	logger := NewCollapseResultsLogger()

	sender, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	recipient, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	accounts := NewAccounts(store.NewInmem())
	WriteAccountBalance(accounts.tree, sender.PublicKey(), 5)

	var txs []*Transaction

	payload, err := Transfer{Recipient: recipient.PublicKey(), Amount: 1}.Marshal()
	if !assert.NoError(t, err) {
		return
	}
	txApplied := NewTransaction(sender, 1, 0, sys.TagTransfer, payload)
	txs = append(txs, &txApplied)

	payload, err = Transfer{Recipient: recipient.PublicKey(), Amount: 10}.Marshal()
	if !assert.NoError(t, err) {
		return
	}
	txRejected := NewTransaction(sender, 2, 0, sys.TagTransfer, payload)
	txs = append(txs, &txRejected)

	block := NewBlock(0, MerkleNodeID{})

	results, err := collapseTransactions(txs, &block, accounts)
	if !assert.NoError(t, err) {
		return
	}

	logCh := make(chan []byte, 2)
	log.SetWriter("tx_write_test", writerFunc(func(p []byte) (n int, err error) {
		logCh <- p
		return len(p), nil
	}))

	logger.Log(results)

	var outputs [][]byte
	// Wait for the log messages
	for i := 0; i < 2; i++ {
		select {
		case b := <-logCh:
			outputs = append(outputs, b)
		case <-time.After(100 * time.Millisecond):
			assert.FailNow(t, "timeout waiting for message", "expected 3 messages, timeout at %d", i)
		}
	}

	// Check the first tx event

	v, err := fastjson.ParseBytes(outputs[0])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "tx", string(v.GetStringBytes("mod")))
	assert.Equal(t, "applied", string(v.GetStringBytes("event")))
	assert.Equal(t, hex.EncodeToString(txApplied.ID[:]), string(v.GetStringBytes("tx_id")))
	assert.Equal(t, hex.EncodeToString(txApplied.Sender[:]), string(v.GetStringBytes("sender_id")))
	assert.Nil(t, v.GetStringBytes("error"))

	// Check the second tx event

	v, err = fastjson.ParseBytes(outputs[1])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "tx", string(v.GetStringBytes("mod")))
	assert.Equal(t, "rejected", string(v.GetStringBytes("event")))
	assert.Equal(t, hex.EncodeToString(txRejected.ID[:]), string(v.GetStringBytes("tx_id")))
	assert.Equal(t, hex.EncodeToString(txRejected.Sender[:]), string(v.GetStringBytes("sender_id")))
	assert.Contains(t, string(v.GetStringBytes("error")), "could not apply transfer transaction")

	// Call twice, the second call should have no effect.
	logger.Stop()
	logger.Stop()

	assert.True(t, logger.closed)
	_, ok := <-logger.flushCh
	assert.False(t, ok)
}

type writerFunc func(p []byte) (n int, err error)

func (w writerFunc) Write(p []byte) (n int, err error) {
	return w(p)
}
