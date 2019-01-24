package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/pkg/errors"
)

type PayloadBuilder struct {
	buffer *bytes.Buffer
}

func NewPayloadBuilder() *PayloadBuilder {
	return &PayloadBuilder{
		buffer: bytes.NewBuffer(nil),
	}
}

func (b *PayloadBuilder) Build() []byte {
	return b.buffer.Bytes()
}

func (b *PayloadBuilder) WriteBytes(x []byte) {
	b.WriteUint32(uint32(len(x)))
	b.buffer.Write(x)
}

func (b *PayloadBuilder) WriteUTF8String(x string) {
	b.buffer.WriteString(x)
	b.buffer.WriteByte(0)
}

func (b *PayloadBuilder) WriteByte(x byte) {
	b.buffer.WriteByte(x)
}

func (b *PayloadBuilder) WriteUint16(x uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], x)
	b.buffer.Write(buf[:])
}

func (b *PayloadBuilder) WriteUint32(x uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], x)
	b.buffer.Write(buf[:])
}

func (b *PayloadBuilder) WriteUint64(x uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], x)
	b.buffer.Write(buf[:])
}

type PayloadReader struct {
	reader *bytes.Reader
}

func NewPayloadReader(payload []byte) *PayloadReader {
	return &PayloadReader{
		reader: bytes.NewReader(payload),
	}
}

func (r *PayloadReader) ReadBytes() ([]byte, error) {
	_xLen, err := r.ReadUint32()
	if err != nil {
		return nil, err
	}
	xLen := int(_xLen)
	if xLen < 0 || xLen > r.reader.Len() {
		return nil, errors.New("bytes out of bounds")
	}
	buf := make([]byte, xLen)
	r.reader.Read(buf)
	return buf, nil
}

func (r *PayloadReader) ReadUTF8String() (string, error) {
	buf := make([]byte, 0)

	for {
		x, err := r.reader.ReadByte()
		if err != nil {
			return "", err
		}
		if x == 0 {
			return string(buf), nil
		}
		buf = append(buf, x)
	}
}

func (r *PayloadReader) ReadByte() (byte, error) {
	return r.reader.ReadByte()
}

func (r *PayloadReader) ReadUint16() (uint16, error) {
	var buf [2]byte
	_, err := r.reader.Read(buf[:])
	return binary.LittleEndian.Uint16(buf[:]), err
}

func (r *PayloadReader) ReadUint32() (uint32, error) {
	var buf [4]byte
	_, err := r.reader.Read(buf[:])
	return binary.LittleEndian.Uint32(buf[:]), err
}

func (r *PayloadReader) ReadUint64() (uint64, error) {
	var buf [8]byte
	_, err := r.reader.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:]), err
}
