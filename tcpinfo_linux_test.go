//go:build linux

package main

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestReadTCPRetransLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		s, err := ln.Accept()
		if err == nil {
			_, _ = io.Copy(io.Discard, s)
			s.Close()
		}
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	const total = 4 << 20
	if _, err := c.Write(make([]byte, total)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	sent, retrans, ok := readTCPRetrans(c)
	if !ok {
		t.Fatal("TCP_INFO 不可用")
	}
	t.Logf("sent=%d retrans=%d", sent, retrans)
	// 内核 4.19+ 以字节计：应等于写入量；旧内核以段计：应大于 0
	if sent != total && (sent == 0 || sent > total) {
		t.Fatalf("sent=%d, want %d", sent, total)
	}
	if retrans != 0 {
		t.Fatalf("loopback retrans=%d", retrans)
	}
}
