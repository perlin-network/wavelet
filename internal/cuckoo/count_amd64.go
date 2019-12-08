// +build amd64

package cuckoo

const Unroll = 32 * 6

func countZeroBytes(buf []byte) uint32

func CountNonzeroBytes(buf []byte) (count uint) {
	l := len(buf) - len(buf)%Unroll

	for _, v := range buf[l:] {
		if v != 0 {
			count++
		}
	}

	return (uint(l) - uint(countZeroBytes(buf))) + count
}
