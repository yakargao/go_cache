/**
* @Author: CiachoG
* @Description：一致性哈希 - 优化版：添加权重支持和虚拟节点优化
*/
package consistenthash

import (
	"hash/crc32"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

// HashFunc 支持自定义哈希函数
type HashFunc func(data []byte) uint32

// Map 一致性哈希环
type Map struct {
	mu       sync.RWMutex
	hash     HashFunc
	replicas int            // 虚节点倍数
	keys     []int          // 哈希环（排序后的虚拟节点）
	hashMap  map[int]string // 虚拟节点哈希 -> 真实节点
	
	// 新增：权重支持
	weights map[string]int  // 节点权重
}

// Option 配置选项
type Option func(*Map)

// WithReplicas 设置虚节点数量
func WithReplicas(replicas int) Option {
	return func(m *Map) {
		m.replicas = replicas
	}
}

// WithHashFunc 设置哈希函数
func WithHashFunc(fn HashFunc) Option {
	return func(m *Map) {
		m.hash = fn
	}
}

// WithWeight 设置节点权重
func WithWeight(weight int) Option {
	return func(m *Map) {
		// 这个选项需要特殊处理，在Add时指定
	}
}

// New 创建一致性哈希环 - 优化：支持更多配置选项
func New(replicas int, fn HashFunc, opts ...Option) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
		weights:  make(map[string]int),
	}
	
	// 应用选项
	for _, opt := range opts {
		opt(m)
	}
	
	// 默认使用CRC32哈希
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	
	return m
}

// NewWithWeight 创建带权重的一致性哈希环
func NewWithWeight(replicas int, weights map[string]int) *Map {
	m := New(replicas, nil)
	m.weights = weights
	return m
}

// Add 添加节点 - 优化：支持权重
func (m *Map) Add(keys ...string) {
	m.AddWithWeight(keys...)
}

// AddWithWeight 添加带权重的节点
func (m *Map) AddWithWeight(keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, key := range keys {
		// 获取权重，默认为1
		weight := m.weights[key]
		if weight <= 0 {
			weight = 1
		}
		
		// 根据权重计算虚拟节点数量
		replicas := m.replicas * weight
		
		for i := 0; i < replicas; i++ {
			// 优化：使用更短的字符串进行哈希
			hash := m.hash([]byte(strconv.Itoa(i) + ":" + key))
			m.keys = append(m.keys, int(hash))
			m.hashMap[int(hash)] = key
		}
	}
	
	sort.Ints(m.keys)
}

// Remove 移除节点
func (m *Map) Remove(keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, key := range keys {
		// 获取权重
		weight := m.weights[key]
		if weight <= 0 {
			weight = 1
		}
		
		replicas := m.replicas * weight
		
		for i := 0; i < replicas; i++ {
			hash := m.hash([]byte(strconv.Itoa(i) + ":" + key))
			idx := sort.SearchInts(m.keys, int(hash))
			if idx < len(m.keys) && m.keys[idx] == int(hash) {
				m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
				delete(m.hashMap, int(hash))
			}
		}
	}
}

// Get 获取对应的节点 - 优化：使用二分查找
func (m *Map) Get(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.keys) == 0 {
		return ""
	}
	
	hash := m.hash([]byte(key))
	
	// 二分查找第一个 >= hash 的位置
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= int(hash)
	})
	
	// 环回到开头
	if idx >= len(m.keys) {
		idx = 0
	}
	
	return m.hashMap[m.keys[idx]]
}

// GetMultiple 获取多个节点 - 优化：避免重复
func (m *Map) GetMultiple(key string, count int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.keys) == 0 || count <= 0 {
		return nil
	}
	
	hash := m.hash([]byte(key))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= int(hash)
	})
	
	var result []string
	seen := make(map[string]bool)
	
	for i := 0; i < len(m.keys) && len(result) < count; i++ {
		curIdx := (idx + i) % len(m.keys)
		node := m.hashMap[m.keys[curIdx]]
		
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
	}
	
	return result
}

// SetWeight 设置节点权重
func (m *Map) SetWeight(key string, weight int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 先移除旧节点
	m.removeKey(key)
	
	// 更新权重
	m.weights[key] = weight
	
	// 重新添加
	m.AddWithWeight(key)
}

// removeKey 内部方法：移除节点
func (m *Map) removeKey(key string) {
	weight := m.weights[key]
	if weight <= 0 {
		weight = 1
	}
	
	replicas := m.replicas * weight
	
	for i := 0; i < replicas; i++ {
		hash := m.hash([]byte(strconv.Itoa(i) + ":" + key))
		idx := sort.SearchInts(m.keys, int(hash))
		if idx < len(m.keys) && m.keys[idx] == int(hash) {
			m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
			delete(m.hashMap, int(hash))
		}
	}
}

// GetReplicas 获取虚拟节点数量
func (m *Map) GetReplicas() int {
	return m.replicas
}

// GetKeys 获取所有虚拟节点
func (m *Map) GetKeys() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	keys := make([]int, len(m.keys))
	copy(keys, m.keys)
	return keys
}

// GetNodeCount 获取真实节点数量
func (m *Map) GetNodeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	nodes := make(map[string]bool)
	for _, node := range m.hashMap {
		nodes[node] = true
	}
	
	return len(nodes)
}

// FnvHash 使用FNV哈希 - 更适合字符串
func FnvHash(data []byte) uint32 {
	h := fnv.New32a()
	h.Write(data)
	return h.Sum32()
}
