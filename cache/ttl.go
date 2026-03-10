// TTL支持 - 添加过期时间功能
package cache

import (
	"sync"
	"time"
)

// TTLValue 带过期时间的值
type TTLValue struct {
	Value     ByteView
	ExpiresAt time.Time
}

// TTLCache 支持TTL的缓存
type TTLCache struct {
	cache map[string]TTLValue
	mu    sync.RWMutex
	timer *time.Ticker
	stop  chan struct{}
}

// NewTTLCache 创建TTL缓存
func NewTTLCache(cleanupInterval time.Duration) *TTLCache {
	tc := &TTLCache{
		cache: make(map[string]TTLValue),
		timer: time.NewTicker(cleanupInterval),
		stop:  make(chan struct{}),
	}
	
	go tc.cleanupWorker()
	return tc
}

// Set 设置带TTL的缓存项
func (tc *TTLCache) Set(key string, value ByteView, ttl time.Duration) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	
	tc.cache[key] = TTLValue{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Get 获取缓存项，如果过期返回false
func (tc *TTLCache) Get(key string) (ByteView, bool) {
	tc.mu.RLock()
	ttlValue, ok := tc.cache[key]
	tc.mu.RUnlock()
	
	if !ok {
		return ByteView{}, false
	}
	
	// 检查是否过期
	if time.Now().After(ttlValue.ExpiresAt) {
		// 异步删除过期项
		go tc.deleteIfExpired(key)
		return ByteView{}, false
	}
	
	return ttlValue.Value, true
}

// deleteIfExpired 检查并删除过期项
func (tc *TTLCache) deleteIfExpired(key string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	
	if ttlValue, ok := tc.cache[key]; ok {
		if time.Now().After(ttlValue.ExpiresAt) {
			delete(tc.cache, key)
		}
	}
}

// cleanupWorker 定期清理过期项
func (tc *TTLCache) cleanupWorker() {
	for {
		select {
		case <-tc.timer.C:
			tc.cleanup()
		case <-tc.stop:
			tc.timer.Stop()
			return
		}
	}
}

// cleanup 清理所有过期项
func (tc *TTLCache) cleanup() {
	now := time.Now()
	
	tc.mu.Lock()
	defer tc.mu.Unlock()
	
	for key, ttlValue := range tc.cache {
		if now.After(ttlValue.ExpiresAt) {
			delete(tc.cache, key)
		}
	}
}

// Delete 删除缓存项
func (tc *TTLCache) Delete(key string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.cache, key)
}

// Len 返回缓存项数量（包括未过期的）
func (tc *TTLCache) Len() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return len(tc.cache)
}

// Clear 清空缓存
func (tc *TTLCache) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.cache = make(map[string]TTLValue)
}

// Close 关闭缓存，停止清理goroutine
func (tc *TTLCache) Close() {
	close(tc.stop)
}

// Stats 返回缓存统计信息
type TTLStats struct {
	TotalItems   int
	ExpiredItems int
}

func (tc *TTLCache) Stats() TTLStats {
	now := time.Now()
	stats := TTLStats{}
	
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	stats.TotalItems = len(tc.cache)
	
	for _, ttlValue := range tc.cache {
		if now.After(ttlValue.ExpiresAt) {
			stats.ExpiredItems++
		}
	}
	
	return stats
}

// TTLGroup 支持TTL的缓存组
type TTLGroup struct {
	*Group
	ttlCache *TTLCache
}

// NewTTLGroup 创建支持TTL的缓存组
func NewTTLGroup(name string, cacheBytes int64, getter Getter, ttl time.Duration) *TTLGroup {
	group := NewGroup(name, cacheBytes, getter)
	
	return &TTLGroup{
		Group:    group,
		ttlCache: NewTTLCache(time.Minute), // 每分钟清理一次
	}
}

// GetWithTTL 获取带TTL的缓存
func (tg *TTLGroup) GetWithTTL(key string, ttl time.Duration) (ByteView, error) {
	// 先检查TTL缓存
	if value, ok := tg.ttlCache.Get(key); ok {
		return value, nil
	}
	
	// 从主缓存获取
	value, err := tg.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	
	// 设置到TTL缓存
	tg.ttlCache.Set(key, value, ttl)
	
	return value, nil
}