// +build !integration,unit

package wavelet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestCollapseResultsLogger(t *testing.T) {
	logger := NewCollapseResultsLogger()

	keys, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	recipient, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	results := &collapseResults{}

	payload, err := Transfer{Recipient: recipient.PublicKey(), Amount: 1}.Marshal()
	if !assert.NoError(t, err) {
		return
	}
	txApplied := NewTransaction(keys, 0, 0, sys.TagTransfer, payload)
	results.appliedCount++
	results.applied = append(results.applied, &txApplied)

	payload, err = Transfer{Recipient: recipient.PublicKey(), Amount: 10}.Marshal()
	if !assert.NoError(t, err) {
		return
	}
	txRejected := NewTransaction(keys, 0, 0, sys.TagTransfer, payload)
	results.rejectedCount++
	results.rejected = append(results.rejected, &txRejected)
	results.rejectedErrors = append(results.rejectedErrors, errors.New("error rejected"))

	logCh := make(chan []byte)
	log.SetWriter("tx", writerFunc(func(p []byte) (n int, err error) {
		logCh <- p
		return len(p), nil
	}))

	logger.Log(results)

	var outputs [][]byte
	// Wait for the log messages
	for i := 0; i < 2; i++ {
		select {
		case b := <-logCh:
			fmt.Println(string(b))
			outputs = append(outputs, b)
		case <-time.After(100 * time.Millisecond):
			assert.FailNow(t, "timeout waiting for message", "expected 2 messages, timeout at %d", i)
		}
	}

	// Check the first tx

	v, err := fastjson.ParseBytes(outputs[0])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "tx", string(v.GetStringBytes("mod")))
	assert.Equal(t, "applied", string(v.GetStringBytes("event")))
	assert.Equal(t, hex.EncodeToString(txApplied.ID[:]), string(v.GetStringBytes("tx_id")))
	assert.Equal(t, hex.EncodeToString(txApplied.Sender[:]), string(v.GetStringBytes("sender_id")))
	assert.Nil(t, v.GetStringBytes("error"))

	// Check the second tx

	v, err = fastjson.ParseBytes(outputs[1])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "tx", string(v.GetStringBytes("mod")))
	assert.Equal(t, "rejected", string(v.GetStringBytes("event")))
	assert.Equal(t, hex.EncodeToString(txRejected.ID[:]), string(v.GetStringBytes("tx_id")))
	assert.Equal(t, hex.EncodeToString(txRejected.Sender[:]), string(v.GetStringBytes("sender_id")))
	assert.Equal(t, "error rejected", string(v.GetStringBytes("error")))

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
