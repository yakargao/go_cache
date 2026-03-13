// 监控指标 - 优化版：使用原子操作替代锁
package cache

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Metrics 缓存指标 - 使用原子操作优化
type Metrics struct {
	// 使用原子操作替代 mutex
	hits        int64
	misses      int64
	evictions   int64
	peerHits    int64
	peerMisses  int64
	peerErrors  int64
	
	// 使用原子操作
	totalGetTime   int64 // 存储纳秒
	getCount       int64
	totalPeerTime  int64
	peerGetCount   int64
	
	// 这些仍然需要锁，因为是整体更新
	currentItems   int64
	currentSize    int64
	maxSize        int64
}

// NewMetrics 创建新的指标收集器
func NewMetrics(maxSize int64) *Metrics {
	return &Metrics{
		maxSize: maxSize,
	}
}

// RecordHit 记录缓存命中 - 原子操作
func (m *Metrics) RecordHit() {
	atomic.AddInt64(&m.hits, 1)
}

// RecordMiss 记录缓存未命中 - 原子操作
func (m *Metrics) RecordMiss() {
	atomic.AddInt64(&m.misses, 1)
}

// RecordEviction 记录缓存淘汰 - 原子操作
func (m *Metrics) RecordEviction() {
	atomic.AddInt64(&m.evictions, 1)
}

// RecordPeerHit 记录从对等节点命中 - 原子操作
func (m *Metrics) RecordPeerHit() {
	atomic.AddInt64(&m.peerHits, 1)
}

// RecordPeerMiss 记录从对等节点未命中 - 原子操作
func (m *Metrics) RecordPeerMiss() {
	atomic.AddInt64(&m.peerMisses, 1)
}

// RecordPeerError 记录对等节点错误 - 原子操作
func (m *Metrics) RecordPeerError() {
	atomic.AddInt64(&m.peerErrors, 1)
}

// RecordGetTime 记录Get操作耗时 - 原子操作
func (m *Metrics) RecordGetTime(duration time.Duration) {
	atomic.AddInt64(&m.totalGetTime, duration.Nanoseconds())
	atomic.AddInt64(&m.getCount, 1)
}

// RecordPeerGetTime 记录从对等节点获取耗时 - 原子操作
func (m *Metrics) RecordPeerGetTime(duration time.Duration) {
	atomic.AddInt64(&m.totalPeerTime, duration.Nanoseconds())
	atomic.AddInt64(&m.peerGetCount, 1)
}

// UpdateSize 更新缓存大小 - 原子操作
func (m *Metrics) UpdateSize(items int64, size int64) {
	atomic.StoreInt64(&m.currentItems, items)
	atomic.StoreInt64(&m.currentSize, size)
}

// Stats 获取当前统计信息 - 优化：减少锁竞争
func (m *Metrics) Stats() MetricsStats {
	// 原子读取所有计数器
	stats := MetricsStats{
		Hits:         atomic.LoadInt64(&m.hits),
		Misses:       atomic.LoadInt64(&m.misses),
		Evictions:    atomic.LoadInt64(&m.evictions),
		PeerHits:     atomic.LoadInt64(&m.peerHits),
		PeerMisses:   atomic.LoadInt64(&m.peerMisses),
		PeerErrors:   atomic.LoadInt64(&m.peerErrors),
		CurrentItems: atomic.LoadInt64(&m.currentItems),
		CurrentSize:  atomic.LoadInt64(&m.currentSize),
		MaxSize:      m.maxSize,
	}
	
	// 计算命中率
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	}
	
	// 计算平均耗时
	getCount := atomic.LoadInt64(&m.getCount)
	if getCount > 0 {
		totalTime := atomic.LoadInt64(&m.totalGetTime)
		stats.AvgGetTime = time.Duration(totalTime / getCount)
	}
	
	peerGetCount := atomic.LoadInt64(&m.peerGetCount)
	if peerGetCount > 0 {
		totalPeerTime := atomic.LoadInt64(&m.totalPeerTime)
		stats.AvgPeerTime = time.Duration(totalPeerTime / peerGetCount)
	}
	
	return stats
}

// ResetStats 重置统计信息
func (m *Metrics) ResetStats() {
	atomic.StoreInt64(&m.hits, 0)
	atomic.StoreInt64(&m.misses, 0)
	atomic.StoreInt64(&m.evictions, 0)
	atomic.StoreInt64(&m.peerHits, 0)
	atomic.StoreInt64(&m.peerMisses, 0)
	atomic.StoreInt64(&m.peerErrors, 0)
	atomic.StoreInt64(&m.totalGetTime, 0)
	atomic.StoreInt64(&m.getCount, 0)
	atomic.StoreInt64(&m.totalPeerTime, 0)
	atomic.StoreInt64(&m.peerGetCount, 0)
}

// MetricsStats 指标统计信息
type MetricsStats struct {
	Hits          int64
	Misses        int64
	Evictions     int64
	PeerHits      int64
	PeerMisses    int64
	PeerErrors    int64
	HitRate       float64
	AvgGetTime    time.Duration
	AvgPeerTime   time.Duration
	CurrentItems  int64
	CurrentSize   int64
	MaxSize       int64
}

// String 返回可读的统计信息
func (s MetricsStats) String() string {
	return s.format()
}

// format 格式化统计信息
func (s MetricsStats) format() string {
	return fmt.Sprintf("Hits: %d, Misses: %d, HitRate: %.2f%%, Items: %d, Size: %d/%d bytes", 
		s.Hits, s.Misses, s.HitRate*100, s.CurrentItems, s.CurrentSize, s.MaxSize)
}

// ToJSON 返回JSON格式的统计信息
func (s MetricsStats) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// InstrumentedGroup 带监控的缓存组
type InstrumentedGroup struct {
	*Group
	metrics *Metrics
}

// NewInstrumentedGroup 创建带监控的缓存组
func NewInstrumentedGroup(name string, cacheBytes int64, getter Getter) *InstrumentedGroup {
	group := NewGroup(name, cacheBytes, getter)
	
	return &InstrumentedGroup{
		Group:   group,
		metrics: NewMetrics(cacheBytes),
	}
}

// Get 重写Get方法以收集指标
func (ig *InstrumentedGroup) Get(key string) (ByteView, error) {
	start := time.Now()
	
	value, err := ig.Group.Get(key)
	
	duration := time.Since(start)
	
	// 记录耗时
	ig.metrics.RecordGetTime(duration)
	
	if err != nil {
		ig.metrics.RecordMiss()
	} else {
		ig.metrics.RecordHit()
	}
	
	return value, err
}

// GetStats 获取统计信息
func (ig *InstrumentedGroup) GetStats() MetricsStats {
	return ig.metrics.Stats()
}

// ResetStats 重置统计信息
func (ig *InstrumentedGroup) ResetStats() {
	ig.metrics.ResetStats()
}

// UpdateSize 更新缓存大小
func (ig *InstrumentedGroup) UpdateSize(items, size int64) {
	ig.metrics.UpdateSize(items, size)
}

// ServeMetrics HTTP handler for metrics
func (ig *InstrumentedGroup) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	stats := ig.GetStats()
	
	w.Header().Set("Content-Type", "application/json")
	
	if r.URL.Query().Get("pretty") == "1" {
		json.NewEncoder(w).Encode(stats)
	} else {
		jsonBytes, _ := stats.ToJSON(); w.Write(jsonBytes)
	}
}

// ServePrometheus 返回 Prometheus 格式的指标
func (ig *InstrumentedGroup) ServePrometheus(w http.ResponseWriter, r *http.Request) {
	stats := ig.GetStats()
	
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	
	// Prometheus 格式
	fmt.Fprintf(w, "# HELP cache_hits_total Total number of cache hits\n")
	fmt.Fprintf(w, "# TYPE cache_hits_total counter\n")
	fmt.Fprintf(w, "cache_hits_total{name=\"%s\"} %d\n\n", ig.Group.name, stats.Hits)
	
	fmt.Fprintf(w, "# HELP cache_misses_total Total number of cache misses\n")
	fmt.Fprintf(w, "# TYPE cache_misses_total counter\n")
	fmt.Fprintf(w, "cache_misses_total{name=\"%s\"} %d\n\n", ig.Group.name, stats.Misses)
	
	fmt.Fprintf(w, "# HELP cache_items Current number of items in cache\n")
	fmt.Fprintf(w, "# TYPE cache_items gauge\n")
	fmt.Fprintf(w, "cache_items{name=\"%s\"} %d\n\n", ig.Group.name, stats.CurrentItems)
	
	fmt.Fprintf(w, "# HELP cache_size_bytes Current cache size in bytes\n")
	fmt.Fprintf(w, "# TYPE cache_size_bytes gauge\n")
	fmt.Fprintf(w, "cache_size_bytes{name=\"%s\"} %d\n", ig.Group.name, stats.CurrentSize)
}
