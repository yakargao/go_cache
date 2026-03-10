/**
* @Author: CiachoG
* @Date: 2020/5/26 15:12
* @Description：提供被其他节点访问的能力
 */
package cache

import (
	"fmt"
	"go_cache/cache/consistenthash"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultBasePath = "/_gocache"
	defaultReplicas = 50
)

// day3: http通信的核心数据结构
// day5:添加节点选择的功能，实现 PeerPicker 接口。
type HTTPPool struct {
	self        string //记录自己的地址，包含主机名/ip和端口
	basePath    string //节点间通讯地址的前缀
	mu          sync.Mutex
	peers       *consistenthash.Map    //实例化一致性哈希
	httpGetters map[string]*httpGetter //key是url，每个远程节点有一个httpGetter
}

func NewHttpPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

func (h *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s]%s", h.self, fmt.Sprintf(format, v...))
}
func (h *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, h.basePath) {
		panic("HTTPPool serving unexpected path:" + r.URL.Path)
	}
	h.Log("%s,%s", r.Method, r.URL.Path)
	// /<basepath>/<groupname>/<key> required
	parts := strings.SplitN(r.URL.Path[len(h.basePath)+1:], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName := parts[0]
	key := parts[1]
	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group:"+groupName, http.StatusNotFound)
		return
	}
	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice()) //缓存值作为 httpResponse 的 body 返回
}

// 实例化一致性哈希算法，添加传入的节点
func (h *HTTPPool) Set(peers ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.peers = consistenthash.New(defaultReplicas, nil)
	h.peers.Add(peers...) //添加传入的节点
	h.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers { //未每个节点创建http客户端
		h.httpGetters[peer] = &httpGetter{baseURL: peer + h.basePath}
	}
}

// 实现PeerPicker 获取对应机器的节点名
func (h *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if peer := h.peers.Get(key); peer != "" && peer != h.self {
		h.Log("Pick peer %s", peer)
		return h.httpGetters[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)

/**----客户端部分-----**/
//httpGetter 实现PeerGetter，客户端具体类
type httpGetter struct {
	baseURL string
}

// 获取远程节点的内容，并返回[]byte
func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	u := fmt.Sprintf("%v%v%v", h.baseURL, url.QueryEscape(group), url.QueryEscape(key))
	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	return bytes, nil
}

var _ PeerGetter = (*httpGetter)(nil)
