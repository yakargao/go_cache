/**
* @Author: CiachoG
* @Date: 2020/5/23 15:22
* @Description：
 */
package lru

import (
	"container/list"
)

// day1：非并发安全的cache结构体
type Cache struct {
	maxBytes  int64      //最大内存
	nBytes    int64      //已使用的内存
	ll        *list.List //双向链表，使用标准库
	cache     map[string]*list.Element
	OnEvicted func(key string, value Value) //记录移除时的回调函数
}

type entry struct { //list的节点，保存key的目的是方便删除对应的映射
	key   string
	value Value
}

// Value interface
type Value interface {
	Len() int
}

func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// 查找
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele) //双向链表队首队尾是相对的，约定front为队尾
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
}

// 删除
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nBytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

// add
func (c *Cache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nBytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nBytes += int64(len(key)) + int64(value.Len())
	}
	//delete
	for c.maxBytes != 0 && c.maxBytes < c.nBytes {
		c.RemoveOldest()
	}
}

// Len the number of cache entries
func (c *Cache) Len() int {
	return c.ll.Len()
}
