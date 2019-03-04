package conflict

type Resolver interface {
	Reset()
	Tick(counts map[interface{}]float64)

	Prefer(id interface{})
	Preferred() interface{}

	Decided() bool
}
