package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
)

func main() {
	TEXT("NumFilled", NOSPLIT, "func(entries []byte) uint")
	Doc("NumFilled returns the total number of filled entries in all buckets in a Cuckoo filter.")
	ptr := Load(Param("entries").Base(), GP64())
	size := Load(Param("entries").Len(), GP64())

	// Initialize 'filled'.
	filled := GP64()
	XORQ(filled, filled)

	// If we finish going through all entries, we're done.
	Label("loop")
	CMPQ(size, Imm(0))
	JE(LabelRef("done"))

	// Check if *ptr = 0
	CMPQ(Mem{Base: ptr}, Imm(0))
	JE(LabelRef("increment"))

	// If *ptr != 0, increment 'filled'.
	INCQ(filled)

	// Increment ptr and decrement size.
	Label("increment")
	INCQ(ptr)
	DECQ(size)
	JMP(LabelRef("loop"))

	Label("done")
	Comment("Store filled to return value.")
	Store(filled, ReturnIndex(0))
	RET()
	Generate()
}
