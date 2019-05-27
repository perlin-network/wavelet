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
