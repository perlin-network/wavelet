package debounce

type Payload struct {
	buf []byte
}

type PayloadOption func(o *Payload)

func parsePayload(oss []PayloadOption) *Payload {
	o := &Payload{}

	for _, os := range oss {
		os(o)
	}

	return o
}

func Bytes(buf []byte) PayloadOption {
	return func(o *Payload) {
		o.buf = buf
	}
}
