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
	"encoding/hex"
	"sync"
	"time"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

// CollapseResultsLogger is used to write CollapseResults to the logger's writers.
//
// It writes directly into the writers without going through zerolog.
// The reason is that, zerolog will write into all the writers, even the writer's
// module does not match with the message's module.
//
// It also has a buffer to prevent blocking, as writing all the transactions in
// a collapse result may take sometime. A collapse result may contain
// tens of thousands of transactions.
type CollapseResultsLogger struct {
	arena      *fastjson.Arena
	timeLayout string // "2006-01-02T15:04:05Z07:00"

	bufTime []byte

	// Buffer for a batch of messages (many transactions)
	bufBatch []logBuffer

	flushCh chan []logBuffer

	stopWg sync.WaitGroup
	stop   chan struct{}
	closed bool
}

type logBuffer struct {
	module  []byte
	message []byte
}

func NewCollapseResultsLogger() *CollapseResultsLogger {
	c := &CollapseResultsLogger{
		arena:      &fastjson.Arena{},
		timeLayout: "2006-01-02T15:04:05Z07:00",
		bufTime:    make([]byte, 0, 64),

		bufBatch: make([]logBuffer, 0, conf.GetBlockTXLimit()/4),
		flushCh:  make(chan []logBuffer, 1024),

		stop: make(chan struct{}),
	}

	c.stopWg.Add(1)

	go func() {
		defer c.stopWg.Done()

		for {
			// Make stop higher priority.
			// To prevent the runtime from repeatedly selecting flush channel when the stop channel has been closed.
			select {
			case <-c.stop:
				return
			default:
			}

			select {
			case b := <-c.flushCh:
				for i := range b {
					_ = log.Write(string(b[i].module), b[i].message)
				}
			case <-c.stop:
				return
			}
		}
	}()

	return c
}

func (c *CollapseResultsLogger) Log(results *collapseResults) {
	timestamp := time.Now()

	modTx := []byte("tx")
	eventApplied := []byte("applied")
	bufTxID := make([]byte, hex.EncodedLen(SizeTransactionID))
	bufAccount := make([]byte, hex.EncodedLen(SizeAccountID))

	for _, tx := range results.applied {
		_ = hex.Encode(bufTxID, tx.ID[:])
		_ = hex.Encode(bufAccount, tx.Sender[:])

		c.addTx(modTx, eventApplied, timestamp, int(tx.Tag), bufTxID, bufAccount, nil)
	}

	eventRejected := []byte("rejected")

	for i, tx := range results.rejected {
		_ = hex.Encode(bufTxID, tx.ID[:])
		_ = hex.Encode(bufAccount, tx.Sender[:])

		c.addTx(modTx, eventRejected, timestamp, int(tx.Tag), bufTxID, bufAccount, results.rejectedErrors[i])
	}

	c.flush()
}

func (c *CollapseResultsLogger) addTx(mod, event []byte,
	timestamp time.Time, tag int,
	txID []byte, sender []byte, logError error) {
	o := c.arena.NewObject()

	o.Set("mod", c.arena.NewStringBytes(mod))
	o.Set("event", c.arena.NewStringBytes(event))
	o.Set("time", c.arena.NewStringBytes(timestamp.AppendFormat(c.bufTime, c.timeLayout)))

	o.Set("tag", c.arena.NewNumberInt(tag))
	o.Set("tx_id", c.arena.NewStringBytes(txID))
	o.Set("sender_id", c.arena.NewStringBytes(sender))

	if logError != nil {
		o.Set("error", c.arena.NewString(logError.Error()))
	}

	// The length of the JSON is 227, not including the error field.
	buf := make([]byte, 0, 256)

	c.bufBatch = append(c.bufBatch, logBuffer{module: mod, message: o.MarshalTo(buf)})

	c.bufTime = c.bufTime[:0]
	c.arena.Reset()
}

func (c *CollapseResultsLogger) flush() {
	c.flushCh <- c.bufBatch

	c.bufBatch = make([]logBuffer, 0, cap(c.bufBatch))
}

func (c *CollapseResultsLogger) Stop() {
	if c.closed {
		return
	}

	close(c.stop)
	c.stopWg.Wait()

	close(c.flushCh)
	c.closed = true
}
