// RDB快照 - Redis兼容的快照格式
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

// RDBHeader RDB文件头
type RDBHeader struct {
	Magic    [5]byte // "REDIS"
	Version  [4]byte // "0001" - "9999"
}

// RDBFooter RDB文件尾
type RDBFooter struct {
	Checksum uint32
}

// RDBEncoder RDB编码器
type RDBEncoder struct {
	writer *bufio.Writer
	crc32  uint32
}

// RDBDecoder RDB解码器
type RDBDecoder struct {
	reader *bufio.Reader
	crc32  uint32
}

// RDBManager RDB管理器
type RDBManager struct {
	mu           sync.RWMutex
	basePath     string
	currentFile  string
	lastSaveTime time.Time
	compression  bool
	encryption   bool
}

// NewRDBManager 创建RDB管理器
func NewRDBManager(basePath string) (*RDBManager, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create RDB directory failed: %v", err)
	}
	
	return &RDBManager{
		basePath:    basePath,
		compression: true,
		encryption:  false,
	}, nil
}

// Save 保存缓存快照
func (rm *RDBManager) Save(cache *cache.Cache) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(rm.basePath, fmt.Sprintf("dump_%s.rdb", timestamp))
	tempFilename := filename + ".tmp"
	
	// 创建临时文件
	file, err := os.Create(tempFilename)
	if err != nil {
		return fmt.Errorf("create RDB file failed: %v", err)
	}
	defer file.Close()
	
	var writer io.Writer = file
	
	// 压缩
	if rm.compression {
		gzWriter := gzip.NewWriter(writer)
		defer gzWriter.Close()
		writer = gzWriter
	}
	
	// 创建编码器
	encoder := NewRDBEncoder(writer)
	
	// 写入文件头
	if err := encoder.WriteHeader(); err != nil {
		return fmt.Errorf("write RDB header failed: %v", err)
	}
	
	// 写入数据库选择器
	if err := encoder.WriteSelectDB(0); err != nil {
		return fmt.Errorf("write DB selector failed: %v", err)
	}
	
	// 写入缓存数据
	if err := encoder.WriteCache(cache); err != nil {
		return fmt.Errorf("write cache data failed: %v", err)
	}
	
	// 写入文件尾
	if err := encoder.WriteFooter(); err != nil {
		return fmt.Errorf("write RDB footer failed: %v", err)
	}
	
	// 刷新缓冲区
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("flush RDB data failed: %v", err)
	}
	
	// 重命名文件
	if err := os.Rename(tempFilename, filename); err != nil {
		return fmt.Errorf("rename RDB file failed: %v", err)
	}
	
	// 更新状态
	rm.currentFile = filename
	rm.lastSaveTime = time.Now()
	
	// 清理旧文件
	go rm.cleanupOldFiles()
	
	return nil
}

// Load 加载缓存快照
func (rm *RDBManager) Load(cache *cache.Cache) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// 查找最新的RDB文件
	files, err := filepath.Glob(filepath.Join(rm.basePath, "dump_*.rdb"))
	if err != nil {
		return fmt.Errorf("find RDB files failed: %v", err)
	}
	
	if len(files) == 0 {
		return errors.New("no RDB files found")
	}
	
	// 选择最新的文件
	latestFile := files[0]
	for _, file := range files[1:] {
		if file > latestFile {
			latestFile = file
		}
	}
	
	// 打开文件
	file, err := os.Open(latestFile)
	if err != nil {
		return fmt.Errorf("open RDB file failed: %v", err)
	}
	defer file.Close()
	
	var reader io.Reader = file
	
	// 解压缩
	if rm.compression {
		gzReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("create gzip reader failed: %v", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	// 创建解码器
	decoder := NewRDBDecoder(reader)
	
	// 读取文件头
	if err := decoder.ReadHeader(); err != nil {
		return fmt.Errorf("read RDB header failed: %v", err)
	}
	
	// 读取数据库选择器
	db, err := decoder.ReadSelectDB()
	if err != nil {
		return fmt.Errorf("read DB selector failed: %v", err)
	}
	
	if db != 0 {
		return fmt.Errorf("unsupported database: %d", db)
	}
	
	// 读取缓存数据
	if err := decoder.ReadCache(cache); err != nil {
		return fmt.Errorf("read cache data failed: %v", err)
	}
	
	// 验证校验和
	if err := decoder.VerifyChecksum(); err != nil {
		return fmt.Errorf("verify checksum failed: %v", err)
	}
	
	rm.currentFile = latestFile
	
	return nil
}

// NewRDBEncoder 创建RDB编码器
func NewRDBEncoder(writer io.Writer) *RDBEncoder {
	return &RDBEncoder{
		writer: bufio.NewWriter(writer),
		crc32:  crc32.ChecksumIEEE([]byte{}),
	}
}

// WriteHeader 写入文件头
func (e *RDBEncoder) WriteHeader() error {
	header := RDBHeader{
		Magic:   [5]byte{'R', 'E', 'D', 'I', 'S'},
		Version: [4]byte{'0', '0', '0', '1'},
	}
	
	if _, err := e.writer.Write(header.Magic[:]); err != nil {
		return err
	}
	if _, err := e.writer.Write(header.Version[:]); err != nil {
		return err
	}
	
	e.updateCRC32(header.Magic[:])
	e.updateCRC32(header.Version[:])
	
	return nil
}

// WriteSelectDB 写入数据库选择器
func (e *RDBEncoder) WriteSelectDB(db int) error {
	// 写入操作码
	if err := e.writer.WriteByte(0xFE); err != nil {
		return err
	}
	
	// 写入数据库编号
	if err := binary.Write(e.writer, binary.LittleEndian, uint8(db)); err != nil {
		return err
	}
	
	e.updateCRC32([]byte{0xFE})
	e.updateCRC32([]byte{uint8(db)})
	
	return nil
}

// WriteCache 写入缓存数据
func (e *RDBEncoder) WriteCache(cache *cache.Cache) error {
	// 这里需要实现缓存数据的序列化
	// 由于cache.Cache的内部结构不可访问，我们需要通过接口获取数据
	// 这是一个简化实现
	
	// 写入结束标记
	if err := e.writer.WriteByte(0xFF); err != nil {
		return err
	}
	
	e.updateCRC32([]byte{0xFF})
	
	return nil
}

// WriteFooter 写入文件尾
func (e *RDBEncoder) WriteFooter() error {
	footer := RDBFooter{
		Checksum: e.crc32,
	}
	
	if err := binary.Write(e.writer, binary.LittleEndian, footer.Checksum); err != nil {
		return err
	}
	
	return nil
}

// Flush 刷新缓冲区
func (e *RDBEncoder) Flush() error {
	return e.writer.Flush()
}

// updateCRC32 更新CRC32校验和
func (e *RDBEncoder) updateCRC32(data []byte) {
	e.crc32 = crc32.Update(e.crc32, crc32.IEEETable, data)
}

// NewRDBDecoder 创建RDB解码器
func NewRDBDecoder(reader io.Reader) *RDBDecoder {
	return &RDBDecoder{
		reader: bufio.NewReader(reader),
		crc32:  crc32.ChecksumIEEE([]byte{}),
	}
}

// ReadHeader 读取文件头
func (d *RDBDecoder) ReadHeader() error {
	var header RDBHeader
	
	if _, err := io.ReadFull(d.reader, header.Magic[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(d.reader, header.Version[:]); err != nil {
		return err
	}
	
	// 验证魔数
	if string(header.Magic[:]) != "REDIS" {
		return errors.New("invalid RDB magic")
	}
	
	d.updateCRC32(header.Magic[:])
	d.updateCRC32(header.Version[:])
	
	return nil
}

// ReadSelectDB 读取数据库选择器
func (d *RDBDecoder) ReadSelectDB() (int, error) {
	opcode, err := d.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	
	if opcode != 0xFE {
		return 0, errors.New("invalid DB selector opcode")
	}
	
	d.updateCRC32([]byte{opcode})
	
	var db uint8
	if err := binary.Read(d.reader, binary.LittleEndian, &db); err != nil {
		return 0, err
	}
	
	d.updateCRC32([]byte{db})
	
	return int(db), nil
}

// ReadCache 读取缓存数据
func (d *RDBDecoder) ReadCache(cache *cache.Cache) error {
	// 这里需要实现缓存数据的反序列化
	// 由于cache.Cache的内部结构不可访问，我们需要通过接口设置数据
	// 这是一个简化实现
	
	// 读取直到结束标记
	for {
		opcode, err := d.reader.ReadByte()
		if err != nil {
			return err
		}
		
		d.updateCRC32([]byte{opcode})
		
		if opcode == 0xFF {
			break // 结束标记
		}
		
		// 这里应该解析键值对并添加到缓存
		// 简化实现：跳过数据
	}
	
	return nil
}

// VerifyChecksum 验证校验和
func (d *RDBDecoder) VerifyChecksum() error {
	var checksum uint32
	if err := binary.Read(d.reader, binary.LittleEndian, &checksum); err != nil {
		return err
	}
	
	if checksum != d.crc32 {
		return errors.New("checksum mismatch")
	}
	
	return nil
}

// updateCRC32 更新CRC32校验和
func (d *RDBDecoder) updateCRC32(data []byte) {
	d.crc32 = crc32.Update(d.crc32, crc32.IEEETable, data)
}

// cleanupOldFiles 清理旧文件
func (rm *RDBManager) cleanupOldFiles() {
	files, err := filepath.Glob(filepath.Join(rm.basePath, "dump_*.rdb"))
	if err != nil {
		return
	}
	
	// 保留最近5个文件
	if len(files) <= 5 {
		return
	}
	
	// 按文件名排序（时间戳顺序）
	for i := 0; i < len(files)-5; i++ {
		os.Remove(files[i])
	}
}

// GetLastSaveTime 获取最后保存时间
func (rm *RDBManager) GetLastSaveTime() time.Time {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.lastSaveTime
}

// GetCurrentFile 获取当前文件
func (rm *RDBManager) GetCurrentFile() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentFile
}

// EnableCompression 启用压缩
func (rm *RDBManager) EnableCompression(enable bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.compression = enable
}

// EnableEncryption 启用加密
func (rm *RDBManager) EnableEncryption(enable bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.encryption = enable
}