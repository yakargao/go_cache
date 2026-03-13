/**
* @Author: CiachoG
* @Date: 2020/5/25 15:34
* @Description：
 */
package cache

type ByteView struct {
	b []byte
}

// NewByteView 创建新的ByteView
func NewByteView(b []byte) ByteView {
	return ByteView{b: cloneBytes(b)}
}

// Len 返回byteView长度,实现value接口
func (v ByteView) Len() int {
	return len(v.b)
}

// ByteSlice返回正式数据的拷贝，只读防止修改
func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}
func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func (v ByteView) String() string {
	return string(v.b)
}
