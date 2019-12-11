package log

import (
	"testing"

	"github.com/valyala/fastjson"
)

func TestValueHex(t *testing.T) {
	v, err := fastjson.Parse(`{"public_key": "2f16b386d4580b9e73bd00467e17e9c4766518d04d08073a3d999279bbe088d1"}`)
	if err != nil {
		t.Fatal(err)
	}

	var dst [32]byte

	if err := ValueHex(v, &dst, "public_key"); err != nil {
		t.Fatal(err)
	}

	if dst == ([32]byte{}) {
		t.Fatal("dst is empty")
	}
}
