package wavelet

import (
	"github.com/djherbis/buffer"
)

type fileBufferPool struct {
	fileSize int64
	filePool buffer.Pool
}

func newFileBufferPool(fileSize int64, dir string) *fileBufferPool {
	filePool := buffer.NewFilePool(fileSize, dir)

	return &fileBufferPool{
		fileSize: fileSize,
		filePool: filePool,
	}
}

// GetUnbounded returns an unbounded buffer.
func (fbp *fileBufferPool) GetUnbounded() buffer.Buffer {
	return buffer.NewPartition(fbp.filePool)
}

// GetBounded creates a bounded buffer of the given size.
func (fbp *fileBufferPool) GetBounded(size int64) (buffer.BufferAt, error) {
	var buffers []buffer.BufferAt

	filesCount := size / fbp.fileSize

	if size%fbp.fileSize > 0 {
		filesCount++
	}

	cleanup := func() {
		for _, f := range buffers {
			_ = fbp.filePool.Put(f)
		}
	}

	for i := 0; i < int(filesCount); i++ {
		file, err := fbp.filePool.Get()
		if err != nil {
			cleanup()
			return nil, err
		}

		buffers = append(buffers, file.(buffer.BufferAt))
	}

	return &boundedFileBuffer{
		Files:    buffers,
		BufferAt: buffer.NewMultiAt(buffers...),
	}, nil
}

func (fbp *fileBufferPool) Put(buf buffer.Buffer) {
	b, ok := buf.(*boundedFileBuffer)
	if ok {
		for _, f := range b.Files {
			_ = fbp.filePool.Put(f)
		}
	}

	buf.Reset()
}

type boundedFileBuffer struct {
	Files []buffer.BufferAt
	buffer.BufferAt
}
