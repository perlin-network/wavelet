//+build ignore

package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
)

var unroll = 6

func main() {
	TEXT("countZeroBytes", NOSPLIT, "func(buf []byte) uint32")
	buf := Mem{Base: Load(Param("buf").Base(), GP64())}
	n := Load(Param("buf").Len(), GP64())

	// Initialize accumulator to 0.
	acc := GP32()
	XORL(acc, acc)

	// Initialize tmp.
	tmp := GP32()

	// Initialize comparator to 0.
	zero256 := YMM()
	VXORPS(zero256, zero256, zero256)

	zero128 := XMM()
	VXORPS(zero128, zero128, zero128)

	// Loop over blocks and process them with vector instructions.
	blockSize := 32 * unroll

	Label("block_loop")
	CMPQ(n, U32(blockSize))
	JL(LabelRef("done"))

	for i := 0; i < unroll; i++ {
		bx := YMM()
		VMOVDQA(buf.Offset(32*i), bx)

		// tmp = (bx == 0)
		VPCMPEQB(bx, zero256, bx)
		VPMOVMSKB(bx, tmp)

		// tmp = number of bits set in comparison mask
		POPCNTL(tmp, tmp)

		// acc += tmp
		ADDL(tmp, acc)
	}

	ADDQ(U32(blockSize), buf.Base)
	SUBQ(U32(blockSize), n)
	JMP(LabelRef("block_loop"))

	Label("done")
	Store(acc, ReturnIndex(0))
	RET()
	Generate()
}
