// 高性能LRU实现 - 对标Redis/Memcached
package lru

import (
	"container/list"
	"sync"
	"sync/atomic"
)

// FastLRU 使用分片锁减少竞争
type FastLRU struct {
	shards []*lruShard
	shardMask uint32
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
		shardCount = 32 // 默认32个分片
	}
	
	// 确保是2的幂，方便位运算
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
	
	return &FastLRU{
		shards:    shards,
		shardMask: uint32(shardCount - 1),
	}
}

// nextPowerOfTwo 返回大于等于n的最小的2的幂
func nextPowerOfTwo(n int) int {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

// getShard 根据key获取对应的分片
func (f *FastLRU) getShard(key string) *lruShard {
	// 使用快速哈希函数
	hash := fastHash(key)
	return f.shards[hash & f.shardMask]
}

// fastHash 简单的快速哈希函数
func fastHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// Get 获取缓存值
func (f *FastLRU) Get(key string) (value Value, ok bool) {
	shard := f.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	
	if ele, hit := shard.cache[key]; hit {
		shard.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
}

// Add 添加缓存项
func (f *FastLRU) Add(key string, value Value) {
	shard := f.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	
	if ele, ok := shard.cache[key]; ok {
		shard.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		shard.nBytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		ele := shard.ll.PushFront(&Entry{key, value})
		shard.cache[key] = ele
		shard.nBytes += int64(len(key)) + int64(value.Len())
	}
	
	// 淘汰旧数据
	for shard.maxBytes != 0 && shard.nBytes > shard.maxBytes {
		f.removeOldest(shard)
	}
}

// removeOldest 移除最久未使用的项
func (f *FastLRU) removeOldest(shard *lruShard) {
	ele := shard.ll.Back()
	if ele != nil {
		shard.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(shard.cache, kv.Key)
		shard.nBytes -= int64(len(kv.Key)) + int64(kv.value.Len())
		if shard.OnEvicted != nil {
			shard.OnEvicted(kv.Key, kv.value)
		}
	}
}

// Len 返回缓存项数量
func (f *FastLRU) Len() int {
	total := 0
	for _, shard := range f.shards {
		shard.mu.RLock()
		total += shard.ll.Len()
		shard.mu.RUnlock()
	}
	return total
}

// Clear 清空缓存
func (f *FastLRU) Clear() {
	for _, shard := range f.shards {
		shard.mu.Lock()
		shard.cache = make(map[string]*list.Element)
		shard.ll.Init()
		shard.nBytes = 0
		shard.mu.Unlock()
	}
}

// Stats 返回缓存统计信息
type CacheStats struct {
	Items      int
	SizeBytes  int64
	ShardCount int
}

func (f *FastLRU) Stats() CacheStats {
	stats := CacheStats{
		ShardCount: len(f.shards),
	}
	
	for _, shard := range f.shards {
		shard.mu.RLock()
		stats.Items += shard.ll.Len()
		stats.SizeBytes += shard.nBytes
		shard.mu.RUnlock()
	}
	
	return stats
}