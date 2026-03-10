// 性能基准测试
package cache

import (
	"testing"
)

func BenchmarkCacheGet(b *testing.B) {
	cache := NewCache(1024*1024, nil)
	cache.Add("key", String("value"))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkCacheAdd(b *testing.B) {
	cache := NewCache(1024*1024*100, nil) // 100MB缓存
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		cache.Add(key, String("value"))
	}
}

func BenchmarkConcurrentGet(b *testing.B) {
	cache := NewCache(1024*1024, nil)
	cache.Add("key", String("value"))
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("key")
		}
	})
}

func BenchmarkConcurrentAdd(b *testing.B) {
	cache := NewCache(1024*1024*100, nil)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			cache.Add(key, String("value"))
			i++
		}
	})
}

func BenchmarkGroupGet(b *testing.B) {
	group := NewGroup("test", 1024*1024, GetterFunc(
		func(key string) ([]byte, error) {
			return []byte("value"), nil
		},
	))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.Get("key")
	}
}

func BenchmarkGroupGetParallel(b *testing.B) {
	group := NewGroup("test", 1024*1024, GetterFunc(
		func(key string) ([]byte, error) {
			return []byte("value"), nil
		},
	))
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			group.Get("key")
		}
	})
}

// 内存分配基准测试
func BenchmarkByteViewClone(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	bv := ByteView{b: data}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bv.Clone()
	}
}

func BenchmarkByteViewString(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	bv := ByteView{b: data}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bv.String()
	}
}

// LRU性能基准测试
func BenchmarkLRUGet(b *testing.B) {
	cache := NewCache(1024*1024, nil)
	
	// 预热缓存
	for i := 0; i < 1000; i++ {
		key := string(rune('a' + i%26))
		cache.Add(key, String("value"))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		cache.Get(key)
	}
}

func BenchmarkLRUAddWithEviction(b *testing.B) {
	cache := NewCache(1024, nil) // 小缓存，会触发淘汰
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%1000))
		cache.Add(key, String("value"))
	}
}

// 单飞模式基准测试
func BenchmarkSingleFlight(b *testing.B) {
	group := NewGroup("test", 1024*1024, GetterFunc(
		func(key string) ([]byte, error) {
			// 模拟耗时操作
			time.Sleep(10 * time.Millisecond)
			return []byte("value"), nil
		},
	))
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			group.Get("key") // 多个goroutine同时请求同一个key
		}
	})
}

// TTL缓存基准测试
func BenchmarkTTLCacheGet(b *testing.B) {
	tc := NewTTLCache(time.Minute)
	tc.Set("key", String("value"), time.Hour)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.Get("key")
	}
}

func BenchmarkTTLCacheSet(b *testing.B) {
	tc := NewTTLCache(time.Minute)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		tc.Set(key, String("value"), time.Hour)
	}
}

// 内存池基准测试
func BenchmarkBufferPool(b *testing.B) {
	pool := NewBufferPool()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(1024)
		pool.Put(buf)
	}
}

func BenchmarkByteViewPool(b *testing.B) {
	pool := NewByteViewPool()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bv := pool.Get()
		pool.Put(bv)
	}
}

// 对比测试：有内存池 vs 无内存池
func BenchmarkWithPool(b *testing.B) {
	pool := NewByteViewPool()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bv := pool.Get()
		bv.b = append(bv.b, []byte("test data")...)
		pool.Put(bv)
	}
}

func BenchmarkWithoutPool(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bv := &ByteView{b: make([]byte, 0, 1024)}
		bv.b = append(bv.b, []byte("test data")...)
	}
}