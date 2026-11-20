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
	offTotalRetrans = 100 // u32 tcpi_total_retrans（段数）
	offDataSegsOut  = 156 // u32 tcpi_data_segs_out，内核 4.6+
	offBytesSent    = 200 // u64 tcpi_bytes_sent，内核 4.19+
	offBytesRetrans = 208 // u64 tcpi_bytes_retrans，内核 4.19+
)

// readTCPRetrans 返回该连接累计发送量与重传量。
// 内核 4.19+ 以字节计；较旧内核退回以段计。两者单位一致即可算比例。
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
	if cerr != nil || serr != 0 {
		return 0, 0, false
	}
	le := binary.NativeEndian
	switch {
	case n >= offBytesRetrans+8:
		return le.Uint64(buf[offBytesSent:]), le.Uint64(buf[offBytesRetrans:]), true
	case n >= offDataSegsOut+4:
		return uint64(le.Uint32(buf[offDataSegsOut:])), uint64(le.Uint32(buf[offTotalRetrans:])), true
	}
	return 0, 0, false
}
