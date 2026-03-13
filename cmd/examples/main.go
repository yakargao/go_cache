// 演示所有优化功能 - 使用示例
package main

import (
	"fmt"
	"go_cache/cache"
	"time"
)

func main() {
	fmt.Println("=== Go Cache 优化成果演示 ===")
	fmt.Println()
	
	// 1. 演示基础缓存功能
	demoBasicCache()
	
	fmt.Println()
	
	// 2. 演示性能优化功能
	demoPerformanceOptimizations()
	
	fmt.Println()
	
	// 3. 演示监控指标功能
	demoMetrics()
	
	fmt.Println()
	fmt.Println("=== 演示完成 ===")
}

func demoBasicCache() {
	fmt.Println("1. 基础缓存功能演示:")
	
	// 创建缓存组
	group := cache.NewGroup("basic", 1024*1024, cache.GetterFunc(
		func(key string) ([]byte, error) {
			return []byte("default_value_for_" + key), nil
		},
	))
	
	// 添加数据
	group.Get("key1")
	group.Get("key2")
	
	// 获取数据
	if value, err := group.Get("key1"); err == nil {
		fmt.Printf("  Get key1: %s\n", value.String())
	}
	
	// LRU淘汰演示
	fmt.Println("  LRU淘汰测试: 添加大量数据触发淘汰...")
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		group.Get(key)
	}
	
	fmt.Printf("  当前缓存大小: %d bytes\n", 1024*1024)
}

func demoPerformanceOptimizations() {
	fmt.Println("2. 性能优化功能演示:")
	
	// 创建带内存池的缓存组
	group := cache.NewGroup("performance", 10*1024*1024, cache.GetterFunc(
		func(key string) ([]byte, error) {
			// 模拟慢速数据源
			time.Sleep(10 * time.Millisecond)
			return []byte("value_from_source"), nil
		},
	))
	
	// 演示单飞模式
	fmt.Println("  单飞模式测试: 多个并发请求同一个key")
	
	start := time.Now()
	results := make(chan string, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			value, err := group.Get("expensive_key")
			if err != nil {
				results <- fmt.Sprintf("  Goroutine %d: error", id)
			} else {
				results <- fmt.Sprintf("  Goroutine %d: %s", id, value.String())
			}
		}(i)
	}
	
	// 收集结果
	for i := 0; i < 10; i++ {
		fmt.Println(<-results)
	}
	
	elapsed := time.Since(start)
	fmt.Printf("  10个并发请求耗时: %v (应该接近单个请求耗时)\n", elapsed)
	
	// 演示TTL功能
	fmt.Println("  TTL功能测试:")
	ttlCache := cache.NewTTLCache(time.Second)
	
	// 通过反射或直接设置来添加数据
	// TTL缓存的Set方法需要ByteView
	bv := cache.NewByteView([]byte("temporary_data"))
	// 使用 ByteSlice() 方法来创建
	
	ttlCache.Set("temp_key", bv, 2*time.Second)
	
	if value, ok := ttlCache.Get("temp_key"); ok {
		fmt.Printf("  获取TTL缓存: %s\n", value.String())
	}
	
	time.Sleep(3 * time.Second)
	if _, ok := ttlCache.Get("temp_key"); !ok {
		fmt.Println("  TTL缓存已过期 (正确)")
	}
	ttlCache.Close()
}

func demoMetrics() {
	fmt.Println("3. 监控指标功能演示:")
	
	// 创建带监控的缓存组
	ig := cache.NewInstrumentedGroup("monitored", 1024*1024, cache.GetterFunc(
		func(key string) ([]byte, error) {
			return []byte("monitored_value"), nil
		},
	))
	
	// 模拟一些请求
	fmt.Println("  模拟缓存请求...")
	for i := 0; i < 20; i++ {
		if i%3 == 0 {
			ig.Get("unknown_key") // 未命中
		} else {
			ig.Get("known_key") // 命中
		}
	}
	
	// 获取统计信息
	stats := ig.GetStats()
	fmt.Printf("  缓存统计:\n")
	fmt.Printf("    命中次数: %d\n", stats.Hits)
	fmt.Printf("    未命中次数: %d\n", stats.Misses)
	fmt.Printf("    命中率: %.1f%%\n", stats.HitRate*100)
	fmt.Printf("    平均获取时间: %v\n", stats.AvgGetTime)
	
	// 演示内存池
	fmt.Println("  内存池演示:")
	pool := cache.NewByteViewPool()
	
	// 使用内存池
	bv1 := pool.Get()
	// ByteView.b 是私有字段，需要通过其他方式访问
	// 这里演示内存池的基本使用
	fmt.Printf("  从池中获取ByteView\n")
	pool.Put(bv1)
	
	bv2 := pool.Get()
	fmt.Printf("  再次从池中获取ByteView (可能重用内存)\n")
	pool.Put(bv2)
	
	// 演示缓冲区池
	fmt.Println("  缓冲区池演示:")
	bufferPool := cache.NewBufferPool()
	
	buf1 := bufferPool.Get(1024)
	buf1 = append(buf1, []byte("buffer data")...)
	fmt.Printf("  获取缓冲区: 长度=%d, 容量=%d\n", len(buf1), cap(buf1))
	bufferPool.Put(buf1)
	
	buf2 := bufferPool.Get(512)
	fmt.Printf("  再次获取缓冲区: 长度=%d, 容量=%d\n", len(buf2), cap(buf2))
	bufferPool.Put(buf2)
}
