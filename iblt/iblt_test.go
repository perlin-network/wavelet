package sync

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	table := NewIBLT(40, 4, 10, 10, 10, nil)
	if !table.isEmpty() {
		t.Fatal()
	}
}

func TestInsertAndDelete(t *testing.T) {
	table := NewIBLT(40, 4, 10, 10, 10, nil)

	assert.True(t, table.isEmpty(), "expected empty")

	table.Insert("key", "value")

	assert.False(t, table.isEmpty(), "expected non empty")

	table.Delete("key", "value")

	assert.True(t, table.isEmpty(), "expected empty")
}

func TestMultiInsert(t *testing.T) {
	table := NewIBLT(40, 4, 10, 10, 10, nil)

	pairs := map[string]string{}
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key::%d", i)
		value := fmt.Sprintf("value::%d", i)
		pairs[key] = value
	}

	for k, v := range pairs {
		table.Insert(k, v)
	}

	entries := table.list()
	for _, e := range entries {
		if val, ok := pairs[e[0]]; !ok || val != e[1] {
			t.Errorf("expected `%v`, got `%v`", val, e[1])
		}
	}

	table.Insert("key", "value")
	pairs["key"] = "value"

	entries = table.list()
	for _, e := range entries {
		if val, ok := pairs[e[0]]; !ok || val != e[1] {
			t.Errorf("expected `%v`, got `%v`", val, e[1])
		}
	}
}

func TestDeleteNone(t *testing.T) {
	table := NewIBLT(40, 4, 10, 10, 10, nil)

	table.Insert("key", "value")

	a, b := table.Get("key")

	t.Logf("`%v` `%v` %v\n", a, b, table.list())
}

func Test_strToInts(t *testing.T) {
	key := "key123"
	keySize := 10

	got := strToInts(key, keySize)
	want := []int{107, 101, 121, 49, 50, 51, 0, 0, 0, 0}

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
