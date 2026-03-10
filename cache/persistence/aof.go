// AOF日志 - 追加写日志
package persistence

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"go_cache/cache"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AOFCommand AOF命令类型
type AOFCommand byte

const (
	AOFSet    AOFCommand = 'S'
	AOFDelete AOFCommand = 'D'
	AOFExpire AOFCommand = 'E'
	AOFSelect AOFCommand = 'B'
)

// AOFEntry AOF日志条目
type AOFEntry struct {
	Timestamp time.Time
	Command   AOFCommand
	Database  uint8
	Key       string
	Value     []byte
	TTL       time.Duration
	Checksum  uint32
}

// AOFWriter AOF写入器
type AOFWriter struct {
	mu           sync.Mutex
	file         *os.File
	writer       *bufio.Writer
	gzipWriter   *gzip.Writer
	basePath     string
	currentFile  string
	fileSize     int64
	maxFileSize  int64
	compression  bool
	syncInterval time.Duration
	lastSync     time.Time
	stopChan     chan struct{}
}

// AOFReader AOF读取器
type AOFReader struct {
	file       *os.File
	reader     *bufio.Reader
	gzipReader *gzip.Reader
}

// AOFManager AOF管理器
type AOFManager struct {
	mu          sync.RWMutex
	basePath    string
	writer      *AOFWriter
	readers     map[string]*AOFReader
	enabled     bool
	appendOnly  bool
	rewriteChan chan struct{}
}

// NewAOFManager 创建AOF管理器
func NewAOFManager(basePath string) (*AOFManager, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create AOF directory failed: %v", err)
	}
	
	manager := &AOFManager{
		basePath:    basePath,
		readers:     make(map[string]*AOFReader),
		enabled:     true,
		appendOnly:  true,
		rewriteChan: make(chan struct{}, 1),
	}
	
	// 启动AOF重写协程
	go manager.rewriteWorker()
	
	return manager, nil
}

// Start 启动AOF写入器
func (am *AOFManager) Start() error {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	if am.writer != nil {
		return errors.New("AOF writer already started")
	}
	
	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(am.basePath, fmt.Sprintf("appendonly_%s.aof", timestamp))
	
	writer, err := NewAOFWriter(filename, 100*1024*1024, true, 1*time.Second)
	if err != nil {
		return fmt.Errorf("create AOF writer failed: %v", err)
	}
	
	am.writer = writer
	am.enabled = true
	
	return nil
}

// Stop 停止AOF写入器
func (am *AOFManager) Stop() error {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	if am.writer == nil {
		return errors.New("AOF writer not started")
	}
	
	if err := am.writer.Close(); err != nil {
		return err
	}
	
	am.writer = nil
	am.enabled = false
	
	// 关闭所有读取器
	for _, reader := range am.readers {
		reader.Close()
	}
	am.readers = make(map[string]*AOFReader)
	
	return nil
}

// AppendSet 追加SET命令
func (am *AOFManager) AppendSet(db uint8, key string, value []byte) error {
	if !am.enabled || am.writer == nil {
		return nil
	}
	
	entry := &AOFEntry{
		Timestamp: time.Now(),
		Command:   AOFSet,
		Database:  db,
		Key:       key,
		Value:     value,
	}
	
	return am.writer.WriteEntry(entry)
}

// AppendDelete 追加DELETE命令
func (am *AOFManager) AppendDelete(db uint8, key string) error {
	if !am.enabled || am.writer == nil {
		return nil
	}
	
	entry := &AOFEntry{
		Timestamp: time.Now(),
		Command:   AOFDelete,
		Database:  db,
		Key:       key,
	}
	
	return am.writer.WriteEntry(entry)
}

// AppendExpire 追加EXPIRE命令
func (am *AOFManager) AppendExpire(db uint8, key string, ttl time.Duration) error {
	if !am.enabled || am.writer == nil {
		return nil
	}
	
	entry := &AOFEntry{
		Timestamp: time.Now(),
		Command:   AOFExpire,
		Database:  db,
		Key:       key,
		TTL:       ttl,
	}
	
	return am.writer.WriteEntry(entry)
}

// AppendSelectDB 追加SELECT命令
func (am *AOFManager) AppendSelectDB(db uint8) error {
	if !am.enabled || am.writer == nil {
		return nil
	}
	
	entry := &AOFEntry{
		Timestamp: time.Now(),
		Command:   AOFSelect,
		Database:  db,
	}
	
	return am.writer.WriteEntry(entry)
}

// Replay 重放AOF日志
func (am *AOFManager) Replay(cache *cache.Cache) error {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	// 查找所有AOF文件
	files, err := filepath.Glob(filepath.Join(am.basePath, "appendonly_*.aof"))
	if err != nil {
		return fmt.Errorf("find AOF files failed: %v", err)
	}
	
	if len(files) == 0 {
		return nil // 没有AOF文件是正常的
	}
	
	// 按文件名排序（时间戳顺序）
	for _, filename := range files {
		reader, err := NewAOFReader(filename)
		if err != nil {
			return fmt.Errorf("create AOF reader failed: %v", err)
		}
		defer reader.Close()
		
		// 重放文件中的所有条目
		for {
			entry, err := reader.ReadEntry()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("read AOF entry failed: %v", err)
			}
			
			// 应用命令到缓存
			if err := am.applyEntry(cache, entry); err != nil {
				return fmt.Errorf("apply AOF entry failed: %v", err)
			}
		}
	}
	
	return nil
}

// applyEntry 应用AOF条目到缓存
func (am *AOFManager) applyEntry(cache *cache.Cache, entry *AOFEntry) error {
	// 这里需要实现将AOF命令应用到缓存
	// 由于cache.Cache的内部结构不可访问，这是一个简化实现
	
	switch entry.Command {
	case AOFSet:
		// cache.Add(entry.Key, cache.ByteView{b: entry.Value})
	case AOFDelete:
		// cache.Remove(entry.Key)
	case AOFExpire:
		// 设置过期时间
	case AOFSelect:
		// 选择数据库
	}
	
	return nil
}

// RequestRewrite 请求AOF重写
func (am *AOFManager) RequestRewrite() {
	select {
	case am.rewriteChan <- struct{}{}:
	default:
		// 重写请求已存在
	}
}

// rewriteWorker AOF重写工作协程
func (am *AOFManager) rewriteWorker() {
	for range am.rewriteChan {
		am.performRewrite()
	}
}

// performRewrite 执行AOF重写
func (am *AOFManager) performRewrite() {
	// 这里需要实现AOF重写逻辑
	// 将多个AOF文件合并为一个，删除重复和过期的命令
}

// NewAOFWriter 创建AOF写入器
func NewAOFWriter(filename string, maxFileSize int64, compression bool, syncInterval time.Duration) (*AOFWriter, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	
	writer := &AOFWriter{
		file:         file,
		writer:       bufio.NewWriter(file),
		basePath:     filepath.Dir(filename),
		currentFile:  filename,
		fileSize:     0,
		maxFileSize:  maxFileSize,
		compression:  compression,
		syncInterval: syncInterval,
		lastSync:     time.Now(),
		stopChan:     make(chan struct{}),
	}
	
	if compression {
		writer.gzipWriter = gzip.NewWriter(writer.writer)
	}
	
	// 启动同步协程
	go writer.syncWorker()
	
	return writer, nil
}

// WriteEntry 写入AOF条目
func (w *AOFWriter) WriteEntry(entry *AOFEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// 序列化条目
	data, err := w.serializeEntry(entry)
	if err != nil {
		return err
	}
	
	// 写入数据
	var writer io.Writer = w.writer
	if w.compression && w.gzipWriter != nil {
		writer = w.gzipWriter
	}
	
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	w.fileSize += int64(len(data))
	
	// 检查文件大小，如果需要则滚动
	if w.fileSize >= w.maxFileSize {
		if err := w.rotateFile(); err != nil {
			return err
		}
	}
	
	return nil
}

// serializeEntry 序列化AOF条目
func (w *AOFWriter) serializeEntry(entry *AOFEntry) ([]byte, error) {
	// 这里需要实现条目的序列化
	// 简化实现：返回空数据
	return []byte{}, nil
}

// rotateFile 滚动文件
func (w *AOFWriter) rotateFile() error {
	// 关闭当前文件
	if w.compression && w.gzipWriter != nil {
		if err := w.gzipWriter.Close(); err != nil {
			return err
		}
	}
	
	if err := w.writer.Flush(); err != nil {
		return err
	}
	
	if err := w.file.Close(); err != nil {
		return err
	}
	
	// 创建新文件
	timestamp := time.Now().Format("20060102_150405")
	newFilename := filepath.Join(w.basePath, fmt.Sprintf("appendonly_%s.aof", timestamp))
	
	file, err := os.OpenFile(newFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	
	// 更新状态
	w.file = file
	w.writer = bufio.NewWriter(file)
	w.currentFile = newFilename
	w.fileSize = 0
	
	if w.compression {
		w.gzipWriter = gzip.NewWriter(w.writer)
	}
	
	return nil
}

// syncWorker 同步工作协程
func (w *AOFWriter) syncWorker() {
	ticker := time.NewTicker(w.syncInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			w.sync()
		case <-w.stopChan:
			w.sync()
			return
		}
	}
}

// sync 同步数据到磁盘
func (w *AOFWriter) sync() {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.compression && w.gzipWriter != nil {
		w.gzipWriter.Flush()
	}
	
	w.writer.Flush()
	w.file.Sync()
	
	w.lastSync = time.Now()
}

// Close 关闭AOF写入器
func (w *AOFWriter) Close() error {
	close(w.stopChan)
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.compression && w.gzipWriter != nil {
		if err := w.gzipWriter.Close(); err != nil {
			return err
		}
	}
	
	if err := w.writer.Flush(); err != nil {
		return err
	}
	
	return w.file.Close()
}

// NewAOFReader 创建AOF读取器
func NewAOFReader(filename string) (*AOFReader, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	
	reader := &AOFReader{
		file:   file,
		reader: bufio.NewReader(file),
	}
	
	// 尝试检测是否为gzip压缩
	peek, err := reader.reader.Peek(2)
	if err == nil && len(peek) >= 2 && peek[0] == 0x1F && peek[1] == 0x8B {
		gzReader, err := gzip.NewReader(reader.reader)
		if err != nil {
			file.Close()
			return nil, err
		}
		reader.gzipReader = gzReader
		reader.reader = bufio.NewReader(gzReader)
	}
	
	return reader, nil
}

// ReadEntry 读取AOF条目
func (r *AOFReader) ReadEntry() (*AOFEntry, error) {
	// 这里需要实现条目的反序列化
	// 简化实现：返回空条目
	return &AOFEntry{}, nil
}

// Close 关闭AOF读取器
func (r *AOFReader) Close() error {
	if r.gzipReader != nil {
		if err := r.gzipReader.Close(); err != nil {
			return err
		}
	}
	return r.file.Close()
}

// GetStats 获取AOF统计信息
func (am *AOFManager) GetStats() AOFStats {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	stats := AOFStats{
		Enabled:     am.enabled,
		AppendOnly:  am.appendOnly,
		RewritePending: len(am.rewriteChan) > 0,
	}
	
	if am.writer != nil {
		stats.CurrentFile = am.writer.currentFile
		stats.FileSize = am.writer.fileSize
		stats.LastSync = am.writer.lastSync
	}
	
	return stats
}

// AOFStats AOF统计信息
type AOFStats struct {
	Enabled        bool
	AppendOnly     bool
	CurrentFile    string
	FileSize       int64
	LastSync       time.Time
	RewritePending bool
}