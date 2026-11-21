package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeTaier 模拟测速服务器：每把 key 只在发出后 ttl 内有效，过期即拒绝。
type fakeTaier struct {
	ttl    time.Duration
	mu     sync.Mutex
	issued map[string]time.Time
	n      int
	queue  int  // 前 queue 次排队请求回「2 排队中」
	busy   bool // 一律回「0 忙」
	asked  int
}

func (f *fakeTaier) valid(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.issued[key]
	return ok && time.Since(t) < f.ttl
}

func (f *fakeTaier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/speed/dovalid":
		if r.URL.Query().Get("key") != "" { // dequeue
			return
		}
		f.mu.Lock()
		f.asked++
		if f.busy {
			f.mu.Unlock()
			fmt.Fprint(w, "0")
			return
		}
		if f.asked <= f.queue {
			f.mu.Unlock()
			fmt.Fprint(w, "2")
			return
		}
		f.n++
		key := "k" + strconv.Itoa(f.n)
		f.issued[key] = time.Now()
		f.mu.Unlock()
		fmt.Fprint(w, "1|"+key)
	case "/speed/File(1G).dl":
		if !f.valid(r.URL.Query().Get("key")) {
			http.Error(w, "expired", http.StatusForbidden)
			return
		}
		buf := make([]byte, 64<<10)
		for {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	case "/speed/doAnalsLoad.do":
		if !f.valid(r.Header.Get("Key")) {
			w.Header().Set("Connection", "close")
			http.Error(w, "expired", http.StatusForbidden)
			return
		}
		// 限速约 500Mbps，避免 loopback 在计时开始前就传完 Content-Length 上限
		buf := make([]byte, 64<<10)
		for {
			if _, err := r.Body.Read(buf); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func startFake(t *testing.T, ttl time.Duration) Server {
	return startFakeWith(t, &fakeTaier{ttl: ttl})
}

func startFakeWith(t *testing.T, f *fakeTaier) Server {
	f.issued = map[string]time.Time{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: f}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return Server{HostIP: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
}

// 每个模式各取新 key：即使 key 只够跑一个模式，单线程与多线程都应测到速度。
func TestFreshKeyPerMode(t *testing.T) {
	if testing.Short() {
		t.Skip("需约 12 秒")
	}
	s := startFake(t, 7*time.Second) // 一个模式约 6 秒，两个模式共用一把 key 会过期
	for _, mode := range []string{"single", "multi"} {
		m, err := runMode(s, "TEST", mode, 1, 250, 4, 2)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		t.Logf("%s: down=%.0f up=%.0f Mbps", mode, m.down, m.up)
		if m.fails() != 0 {
			t.Fatalf("%s 失败: down=%.3f up=%.3f", mode, m.down, m.up)
		}
	}
}

func withQueueTiming(t *testing.T, wait, interval time.Duration) {
	ow, oi := queueWait, queueInterval
	queueWait, queueInterval = wait, interval
	t.Cleanup(func() { queueWait, queueInterval = ow, oi })
}

// 排队中要持续询问直到拿到 key（旧版只等约 1.2 秒）
func TestEnqueueWaitsInQueue(t *testing.T) {
	withQueueTiming(t, 2*time.Second, 50*time.Millisecond)
	f := &fakeTaier{ttl: time.Minute, queue: 10}
	s := startFakeWith(t, f)
	key, err := enqueue(s, "T", 200)
	if err != nil || key == "" {
		t.Fatalf("key=%q err=%v", key, err)
	}
	if f.asked != 11 {
		t.Fatalf("asked=%d, want 11", f.asked)
	}
}

func TestEnqueueReasons(t *testing.T) {
	withQueueTiming(t, 300*time.Millisecond, 50*time.Millisecond)
	cases := []struct {
		f    *fakeTaier
		want string
	}{
		{&fakeTaier{busy: true}, "服务器忙"},
		{&fakeTaier{queue: 1000}, "排队超时"},
	}
	for _, c := range cases {
		_, err := enqueue(startFakeWith(t, c.f), "T", 200)
		if err == nil || err.Error() != c.want {
			t.Errorf("err=%v, want %s", err, c.want)
		}
	}
	// 连不上：用已关闭的端口
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if _, err := enqueue(Server{HostIP: "127.0.0.1", Port: port}, "T", 200); err == nil || err.Error() != "连不上测速服务器" {
		t.Errorf("closed port err=%v", err)
	}
}
