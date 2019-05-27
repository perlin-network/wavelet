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

package snappy

import (
	"io"
	"sync"

	"github.com/golang/snappy"
	"google.golang.org/grpc/encoding"
)

// Name is the name registered for the Snappy compressor.
const Name = "snappy"

func init() {
	encoding.RegisterCompressor(&compressor{})
}

var (
	writerPool sync.Pool
	readerPool sync.Pool
)

type compressor struct{}

func (c *compressor) Name() string {
	return Name
}

func (c *compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	ww := writerPool.Get()

	if ww == nil {
		ww = &writeCloser{Writer: snappy.NewBufferedWriter(w)}
	}

	ww.(*writeCloser).Reset(w)

	return ww.(*writeCloser), nil
}

func (c *compressor) Decompress(r io.Reader) (io.Reader, error) {
	rr := readerPool.Get()

	if rr == nil {
		rr = &reader{Reader: snappy.NewReader(r)}
	}

	rr.(*reader).Reset(r)

	return rr.(*reader), nil
}

type writeCloser struct {
	*snappy.Writer
}

func (w *writeCloser) Close() error {
	defer func() {
		writerPool.Put(w)
	}()

	return w.Writer.Close()
}

type reader struct {
	*snappy.Reader
}

func (r *reader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if err == io.EOF {
		readerPool.Put(r)
	}

	return n, err
}
