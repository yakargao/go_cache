// 高性能LRU实现 - 优化版
package lru

import (
	"container/list"
	"sync"
	"sync/atomic"
	"unsafe"
)

// FastLRU 使用分片锁减少竞争
type FastLRU struct {
	shards []*lruShard
	shardMask uint32
	stats unsafe.Pointer
}

type FastLRUStats struct {
	Hits   int64
	Misses int64
	Gets   int64
}

type lruShard struct {
	mu     sync.RWMutex
	cache  map[string]*list.Element
	ll     *list.List
	nBytes int64
	maxBytes int64
	OnEvicted func(string, Value)
}

// NewFastLRU 创建高性能LRU缓存
func NewFastLRU(maxBytes int64, onEvicted func(string, Value), shardCount int) *FastLRU {
	if shardCount <= 0 {
		shardCount = 32
	}
	shardCount = nextPowerOfTwo(shardCount)
	
	shards := make([]*lruShard, shardCount)
	shardBytes := maxBytes / int64(shardCount)
	
	for i := range shards {
		shards[i] = &lruShard{
			cache:     make(map[string]*list.Element),
			ll:        list.New(),
			maxBytes:  shardBytes,
			OnEvicted: onEvicted,
		}
	}
	
	stats := &FastLRUStats{}
	
	return &FastLRU{
		shards:    shards,
		shardMask: uint32(shardCount - 1),
		stats:     unsafe.Pointer(stats),
	}
}

func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

func (f *FastLRU) getShard(key string) *lruShard {
	hash := fastHash(key)
	return f.shards[hash & f.shardMask]
}

func fastHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// Get 获取缓存值 - 优化：原子统计
func (f *FastLRU) Get(key string) (value Value, ok bool) {
	atomic.AddInt64(&(*FastLRUStats)(f.stats).Gets, 1)
	
	shard := f.getShard(key)
	shard.mu.RLock()
	
	if ele, hit := shard.cache[key]; hit {
		shard.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		value = kv.value
		ok = true
		shard.mu.RUnlock()
		atomic.AddInt64(&(*FastLRUStats)(f.stats).Hits, 1)
		return
	}
	shard.mu.RUnlock()
	atomic.AddInt64(&(*FastLRUStats)(f.stats).Misses, 1)
	return
}

// Add 添加缓存项 - 优化：减少锁持有时间
func (f *FastLRU) Add(key string, value Value) {
	shard := f.getShard(key)
	shard.mu.Lock()
	
	var evictedKey string
	var evictedValue Value
	
	if ele, ok := shard.cache[key]; ok {
		shard.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		shard.nBytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		ele := shard.ll.PushFront(&entry{key: key, value: value})
		shard.cache[key] = ele
		shard.nBytes += int64(len(key)) + int64(value.Len())
	}
	
	for shard.maxBytes != 0 && shard.nBytes > shard.maxBytes {
		ele := shard.ll.Back()
		if ele != nil {
			shard.ll.Remove(ele)
			kv := ele.Value.(*entry)
			delete(shard.cache, kv.key)
			shard.nBytes -= int64(len(kv.key)) + int64(kv.value.Len())
			evictedKey = kv.key
			evictedValue = kv.value
		}
	}
	
	shard.mu.Unlock()
	
	if evictedKey != "" && shard.OnEvicted != nil {
		shard.OnEvicted(evictedKey, evictedValue)
	}
}

func (f *FastLRU) Len() int {
	total := 0
	for _, shard := range f.shards {
		shard.mu.RLock()
		total += shard.ll.Len()
		shard.mu.RUnlock()
	}
	return total
}

func (f *FastLRU) Clear() {
	for _, shard := range f.shards {
		shard.mu.Lock()
		shard.cache = make(map[string]*list.Element)
		shard.ll.Init()
		shard.nBytes = 0
		shard.mu.Unlock()
	}
}

type CacheStats struct {
	Items      int64
	SizeBytes  int64
	ShardCount int
	Gets       int64
	Hits       int64
	Misses     int64
	HitRate    float64
}

func (f *FastLRU) Stats() CacheStats {
	stats := (*FastLRUStats)(f.stats)
	
	gets := atomic.LoadInt64(&stats.Gets)
	hits := atomic.LoadInt64(&stats.Hits)
	misses := atomic.LoadInt64(&stats.Misses)
	
	var hitRate float64
	if gets > 0 {
		hitRate = float64(hits) / float64(gets)
	}
	
	totalItems := int64(0)
	totalSize := int64(0)
	for _, s := range f.shards {
		s.mu.RLock()
		totalItems += int64(s.ll.Len())
		totalSize += s.nBytes
		s.mu.RUnlock()
	}
	
	return CacheStats{
		ShardCount: len(f.shards),
		Gets:       gets,
		Hits:       hits,
		Misses:     misses,
		HitRate:    hitRate,
		Items:      totalItems,
		SizeBytes:  totalSize,
	}
}

func (f *FastLRU) ResetStats() {
	stats := (*FastLRUStats)(f.stats)
	atomic.StoreInt64(&stats.Gets, 0)
	atomic.StoreInt64(&stats.Hits, 0)
	atomic.StoreInt64(&stats.Misses, 0)
}
