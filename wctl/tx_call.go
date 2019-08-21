package wctl

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
)

var ErrNotContract = errors.New("Address is not smart contract")

// Call calls a smart contract function
func (c *Client) Call(recipient [32]byte, fn FunctionCall) (*TxResponse, error) {
	a, err := c.GetAccount(recipient)
	if err != nil {
		return nil, err
	}

	if !a.IsContract {
		return nil, ErrNotContract
	}

	if a.Balance < fn.Amount+fn.GasLimit {
		return nil, ErrInsufficientPerls
	}

	return c.sendTransfer(byte(sys.TagTransfer), fn.toTransfer(recipient))
}

// FunctionCall is the struct containing parameters to call a function.
type FunctionCall struct {
	Name     string
	Amount   uint64
	GasLimit uint64
	Params   [][]byte
}

func NewFunctionCall(name string, params ...[]byte) FunctionCall {
	return FunctionCall{
		Name:   name,
		Params: params,
	}
}

func (fn FunctionCall) toTransfer(recipient [32]byte) wavelet.Transfer {
	t := wavelet.Transfer{
		Recipient: recipient,
		Amount:    fn.Amount,
		GasLimit:  fn.GasLimit,
		FuncName:  []byte(fn.Name),
	}

	for _, p := range fn.Params {
		t.FuncParams = append(t.FuncParams, p...)
	}

	return t
}

func DecodeHex(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func EncodeString(s string) []byte {
	return EncodeBytes([]byte(s))
}

func EncodeBytes(b []byte) []byte {
	var lenbuf = make([]byte, 4)
	binary.LittleEndian.PutUint32(lenbuf, uint32(len(b)))

	var buf bytes.Buffer
	buf.Write(lenbuf)
	buf.Write(b)

	return buf.Bytes()
}

func EncodeByte(u byte) []byte {
	return []byte{u}
}

func EncodeUint8(u uint8) []byte {
	return EncodeByte(byte(u))
}

func EncodeUint16(u uint16) []byte {
	var buf = make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, u)
	return buf
}

func EncodeUint32(u uint32) []byte {
	var buf = make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, u)
	return buf
}

func EncodeUint64(u uint64) []byte {
	var buf = make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, u)
	return buf
}
