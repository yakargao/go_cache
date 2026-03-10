// 持久化管理器 - 整合RDB和AOF
package persistence

import (
	"fmt"
	"go_cache/cache"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistenceMode 持久化模式
type PersistenceMode int

const (
	ModeNone PersistenceMode = iota
	ModeRDB
	ModeAOF
	ModeBoth
)

// PersistenceConfig 持久化配置
type PersistenceConfig struct {
	Mode         PersistenceMode
	BasePath     string
	SaveInterval time.Duration
	SaveChanges  int64
	Compression  bool
	Encryption   bool
}

// DefaultConfig 默认配置
var DefaultConfig = PersistenceConfig{
	Mode:         ModeBoth,
	BasePath:     "./data",
	SaveInterval: 5 * time.Minute,
	SaveChanges:  1000,
	Compression:  true,
	Encryption:   false,
}

// PersistenceManager 持久化管理器
type PersistenceManager struct {
	mu           sync.RWMutex
	config       PersistenceConfig
	cache        *cache.Cache
	rdbManager   *RDBManager
	aofManager   *AOFManager
	lastSaveTime time.Time
	changeCount  int64
	stopChan     chan struct{}
	running      bool
}

// NewPersistenceManager 创建持久化管理器
func NewPersistenceManager(cache *cache.Cache, config PersistenceConfig) (*PersistenceManager, error) {
	if config.BasePath == "" {
		config.BasePath = DefaultConfig.BasePath
	}
	
	// 创建数据目录
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("create data directory failed: %v", err)
	}
	
	pm := &PersistenceManager{
		config:      config,
		cache:       cache,
		changeCount: 0,
		stopChan:    make(chan struct{}),
		running:     false,
	}
	
	// 初始化RDB管理器
	if config.Mode == ModeRDB || config.Mode == ModeBoth {
		rdbManager, err := NewRDBManager(filepath.Join(config.BasePath, "rdb"))
		if err != nil {
			return nil, fmt.Errorf("create RDB manager failed: %v", err)
		}
		pm.rdbManager = rdbManager
		rdbManager.EnableCompression(config.Compression)
		rdbManager.EnableEncryption(config.Encryption)
	}
	
	// 初始化AOF管理器
	if config.Mode == ModeAOF || config.Mode == ModeBoth {
		aofManager, err := NewAOFManager(filepath.Join(config.BasePath, "aof"))
		if err != nil {
			return nil, fmt.Errorf("create AOF manager failed: %v", err)
		}
		pm.aofManager = aofManager
	}
	
	return pm, nil
}

// Start 启动持久化管理器
func (pm *PersistenceManager) Start() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if pm.running {
		return fmt.Errorf("persistence manager already running")
	}
	
	// 加载已有数据
	if err := pm.load(); err != nil {
		return fmt.Errorf("load data failed: %v", err)
	}
	
	// 启动AOF管理器
	if pm.aofManager != nil {
		if err := pm.aofManager.Start(); err != nil {
			return fmt.Errorf("start AOF manager failed: %v", err)
		}
	}
	
	// 启动定期保存协程
	go pm.backgroundWorker()
	
	pm.running = true
	pm.lastSaveTime = time.Now()
	
	return nil
}

// Stop 停止持久化管理器
func (pm *PersistenceManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if !pm.running {
		return fmt.Errorf("persistence manager not running")
	}
	
	// 停止定期保存协程
	close(pm.stopChan)
	
	// 保存当前数据
	if err := pm.save(); err != nil {
		return fmt.Errorf("save data failed: %v", err)
	}
	
	// 停止AOF管理器
	if pm.aofManager != nil {
		if err := pm.aofManager.Stop(); err != nil {
			return fmt.Errorf("stop AOF manager failed: %v", err)
		}
	}
	
	pm.running = false
	
	return nil
}

// load 加载数据
func (pm *PersistenceManager) load() error {
	// 先尝试从RDB加载
	if pm.rdbManager != nil {
		if err := pm.rdbManager.Load(pm.cache); err != nil {
			fmt.Printf("Load from RDB failed: %v\n", err)
		} else {
			fmt.Println("Loaded data from RDB")
			return nil
		}
	}
	
	// 如果RDB加载失败，尝试从AOF重放
	if pm.aofManager != nil {
		if err := pm.aofManager.Replay(pm.cache); err != nil {
			return fmt.Errorf("replay AOF failed: %v", err)
		}
		fmt.Println("Replayed data from AOF")
	}
	
	return nil
}

// save 保存数据
func (pm *PersistenceManager) save() error {
	// 保存到RDB
	if pm.rdbManager != nil {
		if err := pm.rdbManager.Save(pm.cache); err != nil {
			return fmt.Errorf("save to RDB failed: %v", err)
		}
		fmt.Println("Saved data to RDB")
	}
	
	pm.lastSaveTime = time.Now()
	pm.changeCount = 0
	
	return nil
}

// backgroundWorker 后台工作协程
func (pm *PersistenceManager) backgroundWorker() {
	saveTicker := time.NewTicker(pm.config.SaveInterval)
	defer saveTicker.Stop()
	
	for {
		select {
		case <-saveTicker.C:
			pm.autoSave()
		case <-pm.stopChan:
			return
		}
	}
}

// autoSave 自动保存
func (pm *PersistenceManager) autoSave() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// 检查是否需要保存
	needSave := false
	
	// 检查时间间隔
	if time.Since(pm.lastSaveTime) >= pm.config.SaveInterval {
		needSave = true
	}
	
	// 检查变更数量
	if pm.changeCount >= pm.config.SaveChanges {
		needSave = true
	}
	
	if needSave {
		if err := pm.save(); err != nil {
			fmt.Printf("Auto save failed: %v\n", err)
		}
	}
}

// RecordChange 记录数据变更
func (pm *PersistenceManager) RecordChange() {
	pm.mu.Lock()
	pm.changeCount++
	pm.mu.Unlock()
}

// OnSet 处理SET操作
func (pm *PersistenceManager) OnSet(key string, value []byte) {
	pm.RecordChange()
	
	if pm.aofManager != nil {
		// 记录到AOF
		pm.aofManager.AppendSet(0, key, value)
	}
}

// OnDelete 处理DELETE操作
func (pm *PersistenceManager) OnDelete(key string) {
	pm.RecordChange()
	
	if pm.aofManager != nil {
		// 记录到AOF
		pm.aofManager.AppendDelete(0, key)
	}
}

// OnExpire 处理EXPIRE操作
func (pm *PersistenceManager) OnExpire(key string, ttl time.Duration) {
	if pm.aofManager != nil {
		// 记录到AOF
		pm.aofManager.AppendExpire(0, key, ttl)
	}
}

// ManualSave 手动保存
func (pm *PersistenceManager) ManualSave() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	return pm.save()
}

// GetStats 获取统计信息
func (pm *PersistenceManager) GetStats() PersistenceStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	stats := PersistenceStats{
		Mode:         pm.config.Mode,
		Running:      pm.running,
		LastSaveTime: pm.lastSaveTime,
		ChangeCount:  pm.changeCount,
		SaveInterval: pm.config.SaveInterval,
		SaveChanges:  pm.config.SaveChanges,
	}
	
	if pm.rdbManager != nil {
		stats.RDBEnabled = true
		stats.RDBFile = pm.rdbManager.GetCurrentFile()
		stats.RDBLastSave = pm.rdbManager.GetLastSaveTime()
	}
	
	if pm.aofManager != nil {
		aofStats := pm.aofManager.GetStats()
		stats.AOFEnabled = aofStats.Enabled
		stats.AOFFile = aofStats.CurrentFile
		stats.AOFFileSize = aofStats.FileSize
		stats.AOFLastSync = aofStats.LastSync
	}
	
	return stats
}

// PersistenceStats 持久化统计信息
type PersistenceStats struct {
	Mode         PersistenceMode
	Running      bool
	LastSaveTime time.Time
	ChangeCount  int64
	SaveInterval time.Duration
	SaveChanges  int64
	
	// RDB相关
	RDBEnabled bool
	RDBFile    string
	RDBLastSave time.Time
	
	// AOF相关
	AOFEnabled  bool
	AOFFile     string
	AOFFileSize int64
	AOFLastSync time.Time
}

// String 返回可读的统计信息
func (ps PersistenceStats) String() string {
	modeStr := "None"
	switch ps.Mode {
	case ModeRDB:
		modeStr = "RDB"
	case ModeAOF:
		modeStr = "AOF"
	case ModeBoth:
		modeStr = "Both"
	}
	
	return fmt.Sprintf("Mode: %s, Running: %v, Changes: %d, LastSave: %v",
		modeStr, ps.Running, ps.ChangeCount, ps.LastSaveTime.Format("2006-01-02 15:04:05"))
}

// Backup 备份数据
func (pm *PersistenceManager) Backup(backupPath string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// 创建备份目录
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("create backup directory failed: %v", err)
	}
	
	// 保存当前状态到备份文件
	backupFile := filepath.Join(backupPath, fmt.Sprintf("backup_%s.rdb", 
		time.Now().Format("20060102_150405")))
	
	if pm.rdbManager != nil {
		if err := pm.rdbManager.Save(pm.cache); err != nil {
			return fmt.Errorf("create backup failed: %v", err)
		}
		
		// 复制RDB文件到备份目录
		currentFile := pm.rdbManager.GetCurrentFile()
		if currentFile != "" {
			data, err := os.ReadFile(currentFile)
			if err != nil {
				return fmt.Errorf("read RDB file failed: %v", err)
			}
			
			if err := os.WriteFile(backupFile, data, 0644); err != nil {
				return fmt.Errorf("write backup file failed: %v", err)
			}
		}
	}
	
	return nil
}

// Restore 恢复数据
func (pm *PersistenceManager) Restore(backupFile string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if !pm.running {
		return fmt.Errorf("persistence manager not running")
	}
	
	// 停止AOF记录
	if pm.aofManager != nil {
		if err := pm.aofManager.Stop(); err != nil {
			return fmt.Errorf("stop AOF manager failed: %v", err)
		}
	}
	
	// 清空当前缓存
	// pm.cache.Clear() // 需要实现Clear方法
	
	// 从备份文件恢复
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("read backup file failed: %v", err)
	}
	
	// 写入RDB文件
	if pm.rdbManager != nil {
		rdbFile := filepath.Join(pm.config.BasePath, "rdb", 
			fmt.Sprintf("restore_%s.rdb", time.Now().Format("20060102_150405")))
		
		if err := os.WriteFile(rdbFile, data, 0644); err != nil {
			return fmt.Errorf("write RDB file failed: %v", err)
		}
		
		// 从RDB加载
		if err := pm.rdbManager.Load(pm.cache); err != nil {
			return fmt.Errorf("load from RDB failed: %v", err)
		}
	}
	
	// 重新启动AOF记录
	if pm.aofManager != nil {
		if err := pm.aofManager.Start(); err != nil {
			return fmt.Errorf("start AOF manager failed: %v", err)
		}
	}
	
	return nil
}