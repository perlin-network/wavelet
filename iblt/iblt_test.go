package iblt

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func TestBasic(t *testing.T) {
	table := New(40, 4, 10, 10, 10, nil)
	if !table.IsEmpty() {
		t.Fatal()
	}
}

func TestInsertAndDelete(t *testing.T) {
	table := New(40, 4, 10, 10, 10, nil)

	assert.True(t, table.IsEmpty(), "expected empty")

	table.Insert("key", "value")

	assert.False(t, table.IsEmpty(), "expected non empty")

	table.Delete("key", "value")

	assert.True(t, table.IsEmpty(), "expected empty")
}

func TestInsertAndDeleteTxIDs(t *testing.T) {
	table := New(8192, 4, 64, 64, 10, nil)

	var appendList []string
	var deleteList []string

	for i := 0; i < 100; i++ {
		key := RandStringRunes(64)
		appendList = append(appendList, key)

		if rand.Intn(10) < 1 {
			deleteList = append(deleteList, key)
		}

		table.Insert(key, key)
	}

	entries := table.List()
	assert.Equal(t, len(appendList), len(entries))

	//for _, e := range entries {
	//	t.Logf("k: %s v:%s\n", e[0], e[1])
	//}

	for _, key := range deleteList {
		table.Delete(key, key)
	}
}

func TestMultiInsert(t *testing.T) {
	table := New(40, 4, 10, 10, 10, nil)

	pairs := map[string]string{}
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key::%d", i)
		value := fmt.Sprintf("value::%d", i)
		pairs[key] = value
	}

	for k, v := range pairs {
		table.Insert(k, v)
	}

	entries := table.List()
	for _, e := range entries {
		if val, ok := pairs[e[0]]; !ok || val != e[1] {
			t.Errorf("expected `%v`, got `%v`", val, e[1])
		}
	}

	table.Insert("key", "value")
	pairs["key"] = "value"

	entries = table.List()
	for _, e := range entries {
		if val, ok := pairs[e[0]]; !ok || val != e[1] {
			t.Errorf("expected `%v`, got `%v`", val, e[1])
		}
	}
}

func TestHandleEmptiness(t *testing.T) {
	table := New(40, 4, 10, 10, 10, nil)

	pairs := []struct {
		key    string
		value  string
		expect string
		res    LookupResult
	}{
		// {"key", "", "", 1},
		{"", "value", "", 3},
		{"", "", "", 3},
	}

	for _, p := range pairs {
		table.Insert(p.key, p.expect)
	}

	for _, p := range pairs {
		value, res := table.Get(p.key)
		assert.True(t, value == p.expect, "expected `%v`, got `%v`", p.expect, value)
		assert.True(t, res == p.res, "expected `%v`, got `%v`", p.res, res)
	}
}

func TestDiff(t *testing.T) {
	table := New(40, 4, 10, 10, 10, nil)
	other := New(40, 4, 10, 10, 10, nil)

	pairs := genPairs(10)
	for k, v := range pairs {
		table.Insert(k, v)
	}

	pairsToRemove := genPairs(5)
	for k, v := range pairsToRemove {
		other.Insert(k, v)
	}

	table.Diff(other)

	for k := range pairsToRemove {
		val, _ := table.Get(k)
		if val != "" {
			t.Fatalf("`%v` `%v`", k, val)
		}
	}

	for k := range pairs {
		val, _ := table.Get(k)
		if _, ok := pairsToRemove[k]; ok && val != "" {
			t.Fatalf("%v\n", k)
		}
	}
}

func Test_strToInts(t *testing.T) {
	key := "key123"
	keySize := 10

	got := strToInts(key, keySize)
	want := []int{107, 101, 121, 49, 50, 51, 0, 0, 0, 0}

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func Test_strToIntsEmpty(t *testing.T) {
	key := ""
	keySize := 10

	got := strToInts(key, keySize)
	want := []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func Test_sumArrays(t *testing.T) {

	a := []int{107, 101, 121, 49, 50, 51}
	b := []int{107, 101, 256, 49, 50, 51}

	got := sumArrays(a, b)
	want := []int{214, 202, 121, 98, 100, 102}

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func Test_diffInts(t *testing.T) {

	a := []int{107, 101, 121, 400, 50, 51}
	b := []int{107, 101, 256, 200, 50, 51}

	got := diffInts(a, b)
	want := []int{0, 0, -135, 200, 0, 0}

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func Test_intsToStr(t *testing.T) {
	key := []int{107, 101, 121, 49, 50, 51}

	got := intsToStr(key)
	want := "key123"

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func Test_intsToStrEmpty(t *testing.T) {
	key := []int{}

	got := intsToStr(key)
	want := ""

	assert.True(t, reflect.DeepEqual(got, want), "expected %v, got %v", got, want)
}

func genPairs(size int) map[string]string {
	pairs := map[string]string{}

	for i := 0; i < size; i++ {
		key := fmt.Sprintf("key::%s", strconv.Itoa(i))
		value := fmt.Sprintf("value::%s", strconv.Itoa(i))
		pairs[key] = value
	}
	return pairs
}
