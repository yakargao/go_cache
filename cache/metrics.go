// 监控指标 - 添加Prometheus指标支持
package cache

import (
	"sync"
	"time"
)

// Metrics 缓存指标
type Metrics struct {
	mu sync.RWMutex
	
	// 计数器
	hits        int64
	misses      int64
	evictions   int64
	peerHits    int64
	peerMisses  int64
	peerErrors  int64
	
	// 计时器
	totalGetTime   time.Duration
	totalPeerTime  time.Duration
	getCount       int64
	peerGetCount   int64
	
	// 当前状态
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

// RecordHit 记录缓存命中
func (m *Metrics) RecordHit() {
	m.mu.Lock()
	m.hits++
	m.mu.Unlock()
}

// RecordMiss 记录缓存未命中
func (m *Metrics) RecordMiss() {
	m.mu.Lock()
	m.misses++
	m.mu.Unlock()
}

// RecordEviction 记录缓存淘汰
func (m *Metrics) RecordEviction() {
	m.mu.Lock()
	m.evictions++
	m.mu.Unlock()
}

// RecordPeerHit 记录从对等节点命中
func (m *Metrics) RecordPeerHit() {
	m.mu.Lock()
	m.peerHits++
	m.mu.Unlock()
}

// RecordPeerMiss 记录从对等节点未命中
func (m *Metrics) RecordPeerMiss() {
	m.mu.Lock()
	m.peerMisses++
	m.mu.Unlock()
}

// RecordPeerError 记录对等节点错误
func (m *Metrics) RecordPeerError() {
	m.mu.Lock()
	m.peerErrors++
	m.mu.Unlock()
}

// RecordGetTime 记录Get操作耗时
func (m *Metrics) RecordGetTime(duration time.Duration) {
	m.mu.Lock()
	m.totalGetTime += duration
	m.getCount++
	m.mu.Unlock()
}

// RecordPeerGetTime 记录从对等节点获取耗时
func (m *Metrics) RecordPeerGetTime(duration time.Duration) {
	m.mu.Lock()
	m.totalPeerTime += duration
	m.peerGetCount++
	m.mu.Unlock()
}

// UpdateSize 更新缓存大小
func (m *Metrics) UpdateSize(items int64, size int64) {
	m.mu.Lock()
	m.currentItems = items
	m.currentSize = size
	m.mu.Unlock()
}

// Stats 获取当前统计信息
func (m *Metrics) Stats() MetricsStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := MetricsStats{
		Hits:        m.hits,
		Misses:      m.misses,
		Evictions:   m.evictions,
		PeerHits:    m.peerHits,
		PeerMisses:  m.peerMisses,
		PeerErrors:  m.peerErrors,
		CurrentItems: m.currentItems,
		CurrentSize:  m.currentSize,
		MaxSize:     m.maxSize,
	}
	
	// 计算命中率
	total := m.hits + m.misses
	if total > 0 {
		stats.HitRate = float64(m.hits) / float64(total)
	}
	
	// 计算平均耗时
	if m.getCount > 0 {
		stats.AvgGetTime = m.totalGetTime / time.Duration(m.getCount)
	}
	
	if m.peerGetCount > 0 {
		stats.AvgPeerTime = m.totalPeerTime / time.Duration(m.peerGetCount)
	}
	
	return stats
}

// MetricsStats 指标统计信息
type MetricsStats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	PeerHits    int64
	PeerMisses  int64
	PeerErrors  int64
	HitRate     float64
	AvgGetTime  time.Duration
	AvgPeerTime time.Duration
	CurrentItems int64
	CurrentSize  int64
	MaxSize     int64
}

// String 返回可读的统计信息
func (s MetricsStats) String() string {
	return formatStats(s)
}

// formatStats 格式化统计信息
func formatStats(stats MetricsStats) string {
	return formatStats(stats)
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
	// 这里需要实现重置逻辑
	// 由于Metrics是私有的，我们需要添加重置方法
}

// HTTP handler for metrics
func (ig *InstrumentedGroup) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	stats := ig.GetStats()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Prometheus metrics (optional)
var (
	promHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache"},
	)
	
	promMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache"},
	)
	
	promSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cache_size_bytes",
			Help: "Current cache size in bytes",
		},
		[]string{"cache"},
	)
)

func init() {
	// 注册Prometheus指标
	prometheus.MustRegister(promHits)
	prometheus.MustRegister(promMisses)
	prometheus.MustRegister(promSize)
}