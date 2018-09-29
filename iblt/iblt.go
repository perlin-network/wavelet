package iblt

import (
	"crypto/sha1"
	"fmt"
	"io"
	"strconv"
)

// IBLT represents a Invertible Bloom Lookup Tables.
// Based on paper: https://arxiv.org/pdf/1101.2245.pdf
type Table struct {
	// m is amount of cells in underlying lookup table
	m int
	// k is amount of hash functions
	k              int
	keySize        int
	valueSize      int
	hashKeySumSize int

	// hash is function( i, value ), where i is index of hash function
	// and value is value to be hashed
	hash func(int, string) int

	t *table
}

type table struct {
	count    []int
	keys     [][]int
	values   [][]int
	hashKeys [][]int
}

func newTable(size, keySize, valueSize, hashKeySumSize int) *table {
	t := &table{
		count:    make([]int, size),
		keys:     make([][]int, size),
		values:   make([][]int, size),
		hashKeys: make([][]int, size),
	}
	for i := range t.count {
		t.keys[i] = make([]int, keySize)
		t.values[i] = make([]int, valueSize)
		t.hashKeys[i] = make([]int, hashKeySumSize)
	}
	return t
}

func (t *table) copy() *table {
	tt := &table{
		count:    make([]int, len(t.count)),
		keys:     make([][]int, len(t.count)),
		values:   make([][]int, len(t.count)),
		hashKeys: make([][]int, len(t.count)),
	}

	copy(tt.count, t.count)

	for i := range t.count {
		tt.keys[i] = make([]int, len(t.keys[0]))
		copy(tt.keys[i], t.keys[i])

		tt.values[i] = make([]int, len(t.values[0]))
		copy(tt.values[i], t.values[i])

		tt.hashKeys[i] = make([]int, len(t.hashKeys[0]))
		copy(tt.hashKeys[i], t.hashKeys[i])
	}
	return tt
}

// LookupResult represents a result of IBLT query.
type LookupResult int

const (
	lookupGetNoMatch            LookupResult = iota // no_match
	lookupGetMatch                                  // match
	lookupGetDeletedMatch                           // deleted_match
	lookupGetInconclusive                           // inconclusive
	lookupListEntriesComplete                       // complete
	lookupListEntriesIncomplete                     // incomplete
)

// New returns a new instance of IBLT.
func New(m, k, keySize, valueSize, hashKeySumSize int, hash func(int, string) int) *Table {
	s := &Table{
		m:              m,
		k:              k,
		keySize:        keySize,
		valueSize:      valueSize,
		hashKeySumSize: hashKeySumSize,
		t:              newTable(m, keySize, valueSize, hashKeySumSize),
	}

	s.hash = hash
	if hash == nil {
		s.hash = s.defaultHash
	}
	return s
}

func (s *Table) defaultHash(i int, value string) int {
	hashHexLength := 8
	value = fmt.Sprintf("%d%s", i, value)
	h, err := strconv.ParseInt(getKeyHash(value)[:hashHexLength], 16, 64)
	if err != nil {
		panic(err)
	}
	return int(h) % s.m
}

// Insert will add key-value from IBLT.
// see page 24 in paper.
func (s *Table) Insert(key, value string) {
	s.insertAux(s.t, key, value)
}

func (s *Table) insertAux(t *table, key, value string) {
	keyArray := strToInts(key, s.keySize)
	valueArray := strToInts(value, s.valueSize)
	hashArray := strToInts(getKeyHash(key), s.hashKeySumSize)

	indices := map[int]struct{}{}
	for i := 0; i < s.k; i++ {
		indices[s.hash(i, key)] = struct{}{}
	}

	for j := range indices {
		t.count[j]++
		t.keys[j] = sumArrays(t.keys[j], keyArray)
		t.values[j] = sumArrays(t.values[j], valueArray)
		t.hashKeys[j] = sumArrays(t.hashKeys[j], hashArray)
	}
}

// Delete will remove key-value from IBLT.
// see page 24 in paper.
func (s *Table) Delete(key, value string) {
	s.deleteAux(s.t, key, value)
}

func (s *Table) deleteAux(t *table, key, value string) {
	keyArray := strToInts(key, s.keySize)
	valueArray := strToInts(value, s.valueSize)
	hashArray := strToInts(getKeyHash(key), s.hashKeySumSize)

	indices := map[int]struct{}{}
	for i := 0; i < s.k; i++ {
		indices[s.hash(i, key)] = struct{}{}
	}

	for j := range indices {
		t.count[j]--
		t.keys[j] = diffInts(t.keys[j], keyArray)
		t.values[j] = diffInts(t.values[j], valueArray)
		t.hashKeys[j] = diffInts(t.hashKeys[j], hashArray)
	}
}

// Get returns a value and look up resulat for a given key.
// Also this implementation handles extraneous deletions.
// see page 24 in paper.
func (s *Table) Get(key string) (string, LookupResult) {

	keyArray := strToInts(key, s.keySize)
	hashArray := strToInts(getKeyHash(key), s.hashKeySumSize)

	for i := 0; i < s.k; i++ {
		j := s.hash(i, key)

		if s.t.count[j] == 0 &&
			isEmpty(s.t.keys[j]) &&
			isEmpty(s.t.hashKeys[j]) {
			return "", lookupGetNoMatch
		}

		if s.t.count[j] == 1 &&
			isEqual(s.t.keys[j], keyArray) &&
			isEqual(s.t.hashKeys[j], hashArray) {
			return intsToStr(s.t.values[j]), lookupGetMatch
		}

		if s.t.count[j] == -1 &&
			isEqual(s.t.keys[j], negateInts(keyArray)) &&
			isEqual(s.t.hashKeys[j], negateInts(hashArray)) {
			return intsToStr(negateInts(s.t.values[j])), lookupGetDeletedMatch
		}
	}
	return "", lookupGetInconclusive
}

func (s *Table) Marshal() ([]byte, error) {
	if s == nil || s.isEmpty() {
		return nil, nil
	}

	p := new(IBLT)

	for _, val := range s.t.count {
		p.Count = append(p.Count, int32(val))
	}

	for _, vals := range s.t.keys {
		values := new(IntArray)

		for _, val := range vals {
			values.Values = append(values.Values, int32(val))
		}

		p.Keys = append(p.Keys, values)
	}

	for _, vals := range s.t.values {
		values := new(IntArray)

		for _, val := range vals {
			values.Values = append(values.Values, int32(val))
		}

		p.Values = append(p.Values, values)
	}

	for _, vals := range s.t.hashKeys {
		values := new(IntArray)

		for _, val := range vals {
			values.Values = append(values.Values, int32(val))
		}

		p.HashKeys = append(p.HashKeys, values)
	}

	return p.Marshal()
}

func (s *Table) Unmarshal(encoded []byte) error {
	p := new(IBLT)

	err := p.Unmarshal(encoded)
	if err != nil {
		return err
	}

	for i, val := range p.Count {
		s.t.count[i] = int(val)
	}

	for i, x := range p.Keys {
		for j, y := range x.Values {
			s.t.keys[i][j] = int(y)
		}
	}

	for i, x := range p.Values {
		for j, y := range x.Values {
			s.t.values[i][j] = int(y)
		}
	}

	for i, x := range p.Values {
		for j, y := range x.Values {
			s.t.hashKeys[i][j] = int(y)
		}
	}

	return nil
}

// Diff in-place removes all entries in other from current IBLT.
func (s *Table) Diff(other *Table) *Table {
	if s == nil || other == nil {
		return nil
	}

	entries := other.list()
	for _, e := range entries {
		s.Delete(e[0], e[1])
	}

	return s
}

func (s *Table) list() [][2]string {
	var res [][2]string

	tt := s.t.copy()

	for i := 0; i < s.m; i++ {
		if tt.count[i] == 1 {
			k := intsToStr(tt.keys[i])
			v := intsToStr(tt.values[i])

			res = append(res, [2]string{k, v})
			s.deleteAux(tt, k, v)
		}
	}
	return res
}

func (s *Table) isEmpty() bool {
	return isEmpty(s.t.count)
}

func sumArrays(arr1, arr2 []int) []int {
	res := make([]int, len(arr1))
	for i := range arr1 {
		res[i] = (arr1[i] + arr2[i]) % 256
	}
	return res
}

func strToInts(value string, length int) []int {
	res := make([]int, length)
	for i := 0; i < length; i++ {
		if i < len(value) {
			res[i] = int(value[i])
		} else {
			res[i] = 0
		}
	}
	return res
}

func intsToStr(arr []int) string {
	size := len(arr)

	if size == 0 {
		return ""
	}

	res := make([]rune, size)
	for i := 0; i < size; i++ {
		res[i] = rune(arr[i])
	}

	k := size - 1
	for i := k; i >= 0; i-- {
		if res[i] != 0 {
			k = i
			break
		}
	}

	return string(res[:k+1])
}

func diffInts(arr1, arr2 []int) []int {
	res := make([]int, len(arr1))
	for i := range arr1 {
		res[i] = (arr1[i] - arr2[i]) % 256
	}
	return res
}

func negateInts(arr []int) []int {
	res := make([]int, len(arr))
	for i := range arr {
		res[i] = (256 - arr[i]) % 256
	}
	return res
}

func isEmpty(arr []int) bool {
	for i := range arr {
		if arr[i] != 0 {
			return false
		}
	}
	return true
}

func isEqual(arr1, arr2 []int) bool {
	for i := range arr1 {
		if arr1[i] != arr2[i] {
			return false
		}
	}
	return true
}

func getKeyHash(value string) string {
	h := sha1.New()
	io.WriteString(h, value)
	s := fmt.Sprintf("%x", h.Sum(nil))
	return s
}
