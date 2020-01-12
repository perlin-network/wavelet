package log

import (
	"testing"

	"github.com/valyala/fastjson"
)

func TestValueHex32(t *testing.T) {
	v, err := fastjson.Parse(`{"public_key": "2f16b386d4580b9e73bd00467e17e9c4766518d04d08073a3d999279bbe088d1"}`)
	if err != nil {
		t.Fatal(err)
	}

	var dst1 [32]byte

	if err := ValueHex(v, &dst1, "public_key"); err != nil {
		t.Fatal(err)
	}

	if dst1 == ([32]byte{}) {
		t.Fatal("dst is empty")
	}

	var dst2 = [32]byte{}

	if err := ValueHex(v, dst2[:], "public_key"); err != nil {
		t.Fatal(err)
	}

	if dst2 == ([32]byte{}) {
		t.Fatal("dst is empty")
	}
}

func TestValueHexSlice(t *testing.T) {
	v, err := fastjson.Parse(`{"public_key": "2f16b386d4580b9e73bd00467e17e9c4766518d04d08073a3d999279bbe088d1"}`)
	if err != nil {
		t.Fatal(err)
	}

	var dst []byte

	if err := ValueHex(v, &dst, "public_key"); err != nil {
		t.Fatal(err)
	}

	if len(dst) == 0 {
		t.Fatal("dst is empty")
	}
}
