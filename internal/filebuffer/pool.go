package filebuffer

import (
	"github.com/djherbis/buffer"
)

type boundedFileBuffer struct {
	Files []buffer.BufferAt
	buffer.BufferAt
}

type Pool struct {
	fileSize int64
	filePool buffer.Pool
}

func NewPool(fileSize int64, dir string) *Pool {
	filePool := buffer.NewFilePool(fileSize, dir)

	return &Pool{
		fileSize: fileSize,
		filePool: filePool,
	}
}

// GetUnbounded returns an unbounded buffer.
func (p *Pool) GetUnbounded() buffer.Buffer {
	return buffer.NewPartition(p.filePool)
}

// GetBounded creates a bounded buffer of the given size.
func (p *Pool) GetBounded(size int64) (buffer.BufferAt, error) {
	var buffers []buffer.BufferAt

	filesCount := size / p.fileSize

	if size%p.fileSize > 0 {
		filesCount++
	}

	cleanup := func() {
		for _, f := range buffers {
			_ = p.filePool.Put(f)
		}
	}

	for i := 0; i < int(filesCount); i++ {
		file, err := p.filePool.Get()
		if err != nil {
			cleanup()
			return nil, err
		}

		buffers = append(buffers, file.(buffer.BufferAt))
	}

	return &boundedFileBuffer{Files: buffers, BufferAt: buffer.NewMultiAt(buffers...)}, nil
}

func (p *Pool) Put(buf buffer.Buffer) {
	b, ok := buf.(*boundedFileBuffer)
	if ok {
		for _, f := range b.Files {
			_ = p.filePool.Put(f)
		}
	}

	buf.Reset()
}
