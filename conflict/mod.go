package conflict

type Item interface {
	Hash() interface{}
}

type Resolver interface {
	Reset()
	Tick(counts map[Item]float64)

	Prefer(id Item)
	Preferred() Item

	Decided() bool
}
