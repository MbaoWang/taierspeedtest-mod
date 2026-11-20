//go:build linux

package main

import (
	"encoding/binary"
	"net"
	"syscall"
	"unsafe"
)

// struct tcp_info（include/uapi/linux/tcp.h）中用到的字段偏移
const (
	offTotalRetrans = 100 // u32 tcpi_total_retrans：累计重传段数
	offSegsOut      = 136 // u32 tcpi_segs_out：内核 4.2+
	offDataSegsOut  = 156 // u32 tcpi_data_segs_out：内核 4.6+
)

// readTCPRetrans 返回该连接累计发出的数据段数与重传段数。
// 与 TcpQuality 相同：分母优先 data_segs_out，为 0 或内核过旧时退回 segs_out。
func readTCPRetrans(c net.Conn) (sent, retrans uint64, ok bool) {
	tc, isTCP := c.(*net.TCPConn)
	if !isTCP {
		return 0, 0, false
	}
	raw, err := tc.SyscallConn()
	if err != nil {
		return 0, 0, false
	}
	var buf [256]byte
	n := uint32(len(buf))
	var serr syscall.Errno
	cerr := raw.Control(func(fd uintptr) {
		_, _, serr = syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, syscall.IPPROTO_TCP, syscall.TCP_INFO,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0)
	})
	if cerr != nil || serr != 0 || n < offSegsOut+4 {
		return 0, 0, false
	}
	ne := binary.NativeEndian
	retrans = uint64(ne.Uint32(buf[offTotalRetrans:]))
	if n >= offDataSegsOut+4 {
		sent = uint64(ne.Uint32(buf[offDataSegsOut:]))
	}
	if sent == 0 {
		sent = uint64(ne.Uint32(buf[offSegsOut:]))
	}
	return sent, retrans, true
}
