/**
* @Author: CiachoG
* @Date: 2020/5/26 20:36
* @Description：
 */
package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// Hash函数,支持自定义
type Hash func(data []byte) uint32

type Map struct {
	hash     Hash
	replicas int            //虚节点的倍数
	keys     []int          //哈希环
	hashMap  map[int]string //虚节点和真实节点的映射
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil { //支持用户自定义，依赖注入的方式，默认为ChecksumIEEE
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

// 添加节点，把节点名进行hash
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicas; i++ {
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key //虚节点的hash对应真实节点的名称
		}
	}
	sort.Ints(m.keys) //排序成环
}

// 缓存名进行hash，获取对应的机器
func (m *Map) Get(key string) string {
	if len(m.keys) == 0 {
		return ""
	}
	hash := int(m.hash([]byte(key))) //32 bit
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})
	return m.hashMap[m.keys[idx%len(m.keys)]] //%把线性数组看成环
}
