/**
* @Author: CiachoG
* @Date: 2020/5/25 15:42
* @Description：
 */
package cache

import (
	"go_cache/cache/lru"
	"sync"
)

// 实例化lru，添加互斥锁mu
type cache struct {
	mu         sync.Mutex
	lru        *lru.Cache
	cacheBytes int64
}

func (c *cache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value)
}
func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	}
	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}

// Cache 接口 - 用于持久化模块
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	Del(key string)
	Len() int
}
