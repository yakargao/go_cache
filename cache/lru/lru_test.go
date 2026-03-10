package lru

import (
	"reflect"
	"testing"
)

type String string

// implement the interface of Value
func (s String) Len() int {
	return len(s)
}

func TestCache_Get(t *testing.T) {
	lru := New(int64(0), nil)
	lru.Add("k1", String("1234"))
	t.Log(lru.cache["k1"].Value)
	if v, ok := lru.Get("k1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit k1 = 1234 failed")
	} else {
		t.Log("the value of k1 is ", v)
	}
	if _, ok := lru.Get("k2"); ok {
		t.Fatalf("cache miss k2 failed")
	}

}
func TestCache_RemoveOldest(t *testing.T) {
	k1, k2, k3 := "k1", "k2", "k3"
	v1, v2, v3 := "value1", "value2", "value3"
	cap := len(k1 + k2 + v1 + v2)
	lru := New(int64(cap), nil)
	lru.Add(k1, String(v1))
	lru.Add(k2, String(v2))
	lru.Add(k3, String(v3))
	t.Log("len of lru", lru.nBytes)
	if _, ok := lru.Get("k1"); ok {
		t.Fatalf("remove k1 failed")
	}

}

func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	lru := New(int64(10), callback)
	lru.Add("key1", String("123456"))
	lru.Add("k2", String("k2"))
	lru.Add("k3", String("k3"))
	lru.Add("k4", String("k4"))

	expect := []string{"key1", "k2"}

	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s", expect)
	}
}
