// 内存池优化 - 减少GC压力
package cache

import (
	"sync"
)

// ByteViewPool 字节视图内存池
type ByteViewPool struct {
	pool sync.Pool
}

// NewByteViewPool 创建字节视图内存池
func NewByteViewPool() *ByteViewPool {
	return &ByteViewPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &ByteView{b: make([]byte, 0, 1024)} // 预分配1KB
			},
		},
	}
}

// Get 从池中获取ByteView
func (p *ByteViewPool) Get() *ByteView {
	return p.pool.Get().(*ByteView)
}

// Put 将ByteView放回池中
func (p *ByteViewPool) Put(bv *ByteView) {
	if bv == nil {
		return
	}
	// 如果容量过大，不回收，避免内存占用过高
	if cap(bv.b) > 64*1024 { // 超过64KB不回收
		return
	}
	// 重置切片但不释放底层数组
	bv.b = bv.b[:0]
	p.pool.Put(bv)
}

// BufferPool 通用缓冲区池
type BufferPool struct {
	pools []sync.Pool
}

// NewBufferPool 创建缓冲区池
func NewBufferPool() *BufferPool {
	bp := &BufferPool{
		pools: make([]sync.Pool, 17), // 2^0 到 2^16
	}
	
	// 初始化不同大小的池
	for i := range bp.pools {
		size := 1 << uint(i)
		bp.pools[i] = sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, size)
			},
		}
	}
	
	return bp
}

// Get 获取合适大小的缓冲区
func (bp *BufferPool) Get(size int) []byte {
	// 找到最接近的2的幂
	idx := 0
	for size > (1 << uint(idx)) {
		idx++
		if idx >= len(bp.pools) {
			// 超过最大预分配大小，直接分配
			return make([]byte, 0, size)
		}
	}
	
	buf := bp.pools[idx].Get().([]byte)
	return buf[:0] // 重置长度
}

// Put 将缓冲区放回池中
func (bp *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}
	
	capacity := cap(buf)
	idx := 0
	for capacity > (1 << uint(idx)) {
		idx++
		if idx >= len(bp.pools) {
			return // 超过最大大小，不回收
		}
	}
	
	// 检查是否是我们分配的大小
	if capacity == (1 << uint(idx)) {
		bp.pools[idx].Put(buf[:0]) // 重置长度后放回
	}
}

// GlobalPools 全局内存池
var (
	byteViewPool = NewByteViewPool()
	bufferPool   = NewBufferPool()
)

// GetByteViewFromPool 从全局池获取ByteView
func GetByteViewFromPool() *ByteView {
	return byteViewPool.Get()
}

// PutByteViewToPool 将ByteView放回全局池
func PutByteViewToPool(bv *ByteView) {
	byteViewPool.Put(bv)
}

// GetBufferFromPool 从全局池获取缓冲区
func GetBufferFromPool(size int) []byte {
	return bufferPool.Get(size)
}

// PutBufferToPool 将缓冲区放回全局池
func PutBufferToPool(buf []byte) {
	bufferPool.Put(buf)
}