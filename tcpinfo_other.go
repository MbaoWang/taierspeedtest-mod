//go:build !linux

package main

import "net"

func readTCPRetrans(c net.Conn) (sent, retrans uint64, ok bool) { return 0, 0, false }
