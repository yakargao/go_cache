# Go Cache 分布式缓存系统

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.18+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge">
  <img src="https://img.shields.io/badge/Version-v1.0.2-blue?style=for-the-badge">
</p>

一个基于 Go 语言实现的分布式缓存系统，支持内存缓存、LRU 淘汰、一致性哈希、分布式部署、性能优化、数据持久化等功能。

## ✨ 特性

### 核心功能
- 🔥 **LRU 缓存** - 基于双向链表和哈希表实现的高效 LRU 淘汰算法
- 🌐 **分布式支持** - 基于一致性哈希的分布式缓存，支持多节点部署
- ⚡ **HTTP 通信** - 基于 HTTP 的节点间通信机制
- 🔄 **单飞模式 (Singleflight)** - 防止缓存击穿，多次请求合并为一次

### 性能优化
- 🏎️ **分片 LRU** - 分片锁减少锁竞争，并发性能提升 4 倍
- 🧠 **内存池** - ByteView 内存池 + 缓冲区池，减少 70% 小对象分配
- ⏱️ **TTL 支持** - 带过期时间的缓存，自动清理过期数据
- 📊 **监控指标** - 完整的性能指标收集，命中率统计

### 数据持久化 (开发中)
- 💾 **RDB 快照** - 定期将内存数据序列化到磁盘
- 📝 **AOF 日志** - 记录所有写操作，支持日志重放
- 🔄 **故障恢复** - 启动时自动恢复数据

## 📁 项目结构

```
go_cache/
├── cache/                      # 核心缓存模块
│   ├── lru/                    # LRU 实现
│   │   ├── lru.go             # 基础 LRU
│   │   └── fast_lru.go        # 高性能分片 LRU
│   ├── persistence/            # 数据持久化
│   │   ├── rdb.go             # RDB 快照
│   │   ├── aof.go            # AOF 日志
│   │   └── manager.go        # 持久化管理器
│   ├── singleflight/          # 单飞模式
│   ├── consistenthash/        # 一致性哈希
│   ├── pool.go               # 内存池
│   ├── ttl.go                # TTL 支持
│   ├── metrics.go            # 监控指标
│   ├── byteview.go           # 字节视图
│   ├── cache.go              # 缓存接口
│   ├── gocache.go           # 缓存组
│   ├── http.go               # HTTP 传输
│   └── peers.go              # 节点接口
├── assets/                    # 静态资源
├── main.go                   # 主程序入口
├── demo_optimizations.go     # 优化演示程序
├── go.mod                    # Go 模块
└── README.md                 # 项目文档
```

## 🚀 快速开始

### 安装

```bash
git clone https://github.com/yakargao/go_cache.git
cd go_cache
go build -o go_cache .
```

### 运行演示

```bash
# 运行优化演示程序
go run demo_optimizations.go

# 运行分布式缓存示例
go run main.go -port=8001
```

### 分布式部署

```bash
# 启动节点 1 (端口 8001)
go run main.go -port=8001

# 启动节点 2 (端口 8002)
go run main.go -port=8002

# 启动节点 3 (端口 8003)
go run main.go -port=8003

# 启动 API 服务器
go run main.go -api
```

## 📖 使用示例

### 基础缓存使用

```go
package main

import (
    "fmt"
    "go_cache/cache"
)

func main() {
    // 创建缓存 (1MB)
    c := cache.NewCache(1024*1024, nil)
    
    // 添加数据
    c.Add("key1", cache.String("value1"))
    c.Add("key2", cache.String("value2"))
    
    // 获取数据
    if value, ok := c.Get("key1"); ok {
        fmt.Printf("key1 = %s\n", value.String())
    }
}
```

### 使用缓存组 (带回调)

```go
package main

import (
    "fmt"
    "go_cache/cache"
)

func main() {
    // 创建缓存组，带数据源回调
    group := cache.NewGroup("scores", 1024*1024, cache.GetterFunc(
        func(key string) ([]byte, error) {
            // 从数据库或其他服务获取数据
            return []byte("data_from_db"), nil
        },
    ))
    
    // 获取数据 (未命中时会调用回调)
    value, err := group.Get("key")
    if err != nil {
        fmt.Println(err)
    }
    fmt.Println(value.String())
}
```

### 使用 TTL 缓存

```go
package main

import (
    "fmt"
    "time"
    "go_cache/cache"
)

func main() {
    // 创建 TTL 缓存
    ttlCache := cache.NewTTLCache(time.Second)
    
    // 设置带过期时间的数据
    ttlCache.Set("temp_key", cache.String("temporary_data"), 2*time.Second)
    
    // 获取数据
    if value, ok := ttlCache.Get("temp_key"); ok {
        fmt.Printf("temp_key = %s\n", value.String())
    }
    
    // 等待过期
    time.Sleep(3 * time.Second)
    
    // 数据已过期
    if _, ok := ttlCache.Get("temp_key"); !ok {
        fmt.Println("数据已过期")
    }
    
    ttlCache.Close()
}
```

### 使用带监控的缓存组

```go
package main

import (
    "fmt"
    "go_cache/cache"
)

func main() {
    // 创建带监控的缓存组
    ig := cache.NewInstrumentedGroup("monitored", 1024*1024, cache.GetterFunc(
        func(key string) ([]byte, error) {
            return []byte("value"), nil
        },
    ))
    
    // 模拟请求
    for i := 0; i < 20; i++ {
        if i%3 == 0 {
            ig.Get("unknown_key") // 未命中
        } else {
            ig.Get("known_key") // 命中
        }
    }
    
    // 获取统计信息
    stats := ig.GetStats()
    fmt.Printf("命中次数: %d\n", stats.Hits)
    fmt.Printf("未命中次数: %d\n", stats.Misses)
    fmt.Printf("命中率: %.1f%%\n", stats.HitRate*100)
}
```

### 使用内存池

```go
package main

import (
    "fmt"
    "go_cache/cache"
)

func main() {
    // 创建 ByteView 内存池
    pool := cache.NewByteViewPool()
    
    // 从池中获取
    bv1 := pool.Get()
    bv1.b = append(bv1.b, []byte("data from pool")...)
    fmt.Printf("Pool data: %s\n", bv1.String())
    pool.Put(bv1)
    
    // 创建缓冲区池
    bufferPool := cache.NewBufferPool()
    buf := bufferPool.Get(1024)
    buf = append(buf, []byte("buffer data")...)
    fmt.Printf("Buffer: len=%d, cap=%d\n", len(buf), cap(buf))
    bufferPool.Put(buf)
}
```

## 📊 性能对比

| 场景 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| 并发 Get (8 goroutines) | ~50k ops/sec | ~200k ops/sec | **4x** |
| 内存分配 (小对象) | ~500 ns/op | ~150 ns/op | **3.3x** |
| 内存使用 | 100% | ~70% | **-30%** |

## 🔧 配置说明

### 主程序参数

```bash
-port=8001     # 缓存服务器端口
-api           # 启动 API 服务器
-demo          # 运行演示模式
```

### 持久化配置

```go
config := persistence.DefaultConfig
config.BasePath = "./data"        // 数据存储路径
config.SaveInterval = 60 * time.Second  // 保存间隔
config.SaveChanges = 1000        // 变更次数触发保存
config.EnableAOF = true          // 启用 AOF
config.AOFFsync = "every"        // AOF 同步策略
```

## 📈 路线图

- [x] v1.0 - 基础分布式缓存
- [x] v1.0.1 - 性能优化 (分片 LRU、内存池)
- [x] v1.0.2 - 可观测性 (监控指标、基准测试)
- [ ] v1.1 - 数据持久化 (RDB/AOF)
- [ ] v1.2 - 协议优化 (Redis 协议兼容)
- [ ] v1.3 - 集群管理 (Raft 共识算法)
- [ ] v2.0 - 高级数据结构

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

---

**当前版本**: v1.0.2  
**最后更新**: 2026-03-14
