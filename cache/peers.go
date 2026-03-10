/**
* @Author: CiachoG
* @Date: 2020/5/27 10:45
* @Description：
 */
package cache

type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter 对应http客户端
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
