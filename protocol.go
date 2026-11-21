package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	pkgName   = "com.cnspeedtest.globalspeed"
	appName   = "globalspeed"
	randSalt  = "12345555"
	uaDalvik  = "Dalvik/2.1.0 (Linux; U; Android 14; NE2210 Build/TP1A.220624.014)"
	uaBrowser = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.60 Safari/537.36"
	uaUpload  = "Dalvik/1.6.0 (Linux; U; Android 4.2.2; GT-I9505 Build/JDQ39)"
	boundary  = "00content0boundary00"
)

var ctrlServers = []string{
	"https://dlcv2.cnspeedtest.cn:8443",
	"http://dlc.duoweisoft.com:8096",
	"http://dlcv2.duoweisoft.com:8088",
}

type ClientInfo struct {
	IP       string
	Province string
	City     string
	Oper     string
}

type Server struct {
	HostID   string
	HostName string
	HostIP   string
	Port     int
	PName    string
	City     string
	Oper     string
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func enqueueToken(imei, ts string, bandwidth int) string {
	h1 := md5Hex("model=Android&imei=" + imei)
	h2 := md5Hex(fmt.Sprintf("stime=%s&band=%d&rand=%s", ts, bandwidth, randSalt))
	return md5Hex(h1 + h2)
}

func hostPort(ip string, port int) string {
	if strings.Contains(ip, ":") && !strings.HasPrefix(ip, "[") {
		return fmt.Sprintf("[%s]:%d", ip, port)
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func httpGet(raw string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", uaDalvik)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func httpPost(raw string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodPost, raw, strings.NewReader(""))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", uaDalvik)
	req.Header.Set("Charset", "utf-8")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fetchClient() (base string, info ClientInfo, err error) {
	var last error
	for _, b := range ctrlServers {
		body, e := httpGet(b+"/dataServer/getIpLocSP.php", 6*time.Second)
		if e != nil {
			last = e
			continue
		}
		parts := strings.Split(strings.TrimSpace(body), "|")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		info.IP = parts[0]
		if len(parts) > 1 {
			var loc []string
			if json.Unmarshal([]byte(parts[1]), &loc) == nil {
				if len(loc) > 1 {
					info.Province = loc[1]
				}
				if len(loc) > 2 {
					info.City = loc[2]
				}
				if len(loc) > 4 {
					info.Oper = loc[4]
				}
			}
		}
		if info.Oper == "" && len(parts) > 3 {
			info.Oper = parts[3]
		}
		return b, info, nil
	}
	if last == nil {
		last = fmt.Errorf("无法获取出口 IP")
	}
	return "", ClientInfo{}, last
}

func matchServers(base, ip, province, city, oper string, ipv6 bool) ([]Server, error) {
	v := url.Values{}
	v.Set("ip", ip)
	v.Set("network", "4")
	v.Set("province", province)
	v.Set("city", city)
	v.Set("wifioper", oper)
	v.Set("mobileoperid", "")
	if ipv6 {
		v.Set("ipv6", "1")
	} else {
		v.Set("ipv6", "0")
	}
	v.Set("model", "Android")
	v.Set("pkg", pkgName)
	body, err := httpGet(base+"/dataServer/mobilematch_many.php?"+v.Encode(), 10*time.Second)
	if err != nil {
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(body), &arr); err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(arr))
	for _, item := range arr {
		port := 0
		switch p := item["port"].(type) {
		case string:
			fmt.Sscanf(p, "%d", &port)
		case float64:
			port = int(p)
		}
		out = append(out, Server{
			HostID:   fmt.Sprint(item["hostid"]),
			HostName: fmt.Sprint(item["hostname"]),
			HostIP:   fmt.Sprint(item["hostip"]),
			Port:     port,
			PName:    fmt.Sprint(item["pname"]),
			City:     fmt.Sprint(item["city"]),
			Oper:     oper,
		})
	}
	return out, nil
}

func tcpOK(ip string, port int) bool {
	c, err := net.DialTimeout("tcp", hostPort(ip, port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func pickServer(servers []Server, province, city, oper string) *Server {
	if len(servers) == 0 {
		return nil
	}
	for i := range servers {
		s := &servers[i]
		if strings.Contains(s.HostName, oper) && (strings.Contains(s.HostName, city) || s.City == city || s.PName == province) {
			if tcpOK(s.HostIP, s.Port) {
				return s
			}
		}
	}
	for i := range servers {
		if tcpOK(servers[i].HostIP, servers[i].Port) {
			return &servers[i]
		}
	}
	return &servers[0]
}

// queueWait 是排队取 key 的最长等待时间；服务器回「排队中」或「忙」时会在此时间内持续询问。
var (
	queueWait     = 15 * time.Second
	queueInterval = time.Second
)

// enqueue 向测速服务器排队取 key。
// 回应：1|key 成功；2 排队中；0 服务器忙；-1 参数错误。
func enqueue(s Server, imei string, bandwidth int) (string, error) {
	deadline := time.Now().Add(queueWait)
	start := time.Now()
	var reason string
	for try := 1; ; try++ {
		ts := fmt.Sprintf("%d", time.Now().Unix()) // token 含时间戳，每次询问都重算
		raw := fmt.Sprintf(
			"http://%s/speed/dovalid?key=&flag=true&bandwidth=%d&model=Android&imei=%s&time=%s&app=%s&token=%s&pkg=%s",
			hostPort(s.HostIP, s.Port), bandwidth, url.QueryEscape(imei), ts, appName, enqueueToken(imei, ts, bandwidth), pkgName,
		)
		body, err := httpGet(raw, 5*time.Second)
		body = strings.TrimSpace(body)
		switch {
		case err != nil:
			reason = "连不上测速服务器"
			debugf("排队 #%d：%v", try, err)
		case strings.HasPrefix(body, "-1"):
			return "", fmt.Errorf("服务器拒绝（参数错误）")
		case strings.HasPrefix(body, "0"):
			reason = "服务器忙"
			debugf("排队 #%d：服务器忙（%s）", try, truncate(body, 60))
		case strings.HasPrefix(body, "2"):
			reason = "排队超时"
			debugf("排队 #%d：排队中（%s）", try, truncate(body, 60))
		case strings.HasPrefix(body, "1") && len(body) > 2:
			debugf("排队 #%d：取得 key，用时 %.1fs", try, time.Since(start).Seconds())
			return body[2:], nil
		default:
			reason = "服务器回应异常"
			debugf("排队 #%d：未知回应 %q", try, truncate(body, 60))
		}
		if time.Now().Add(queueInterval).After(deadline) {
			return "", fmt.Errorf("%s", reason)
		}
		time.Sleep(queueInterval)
	}
}

func dequeue(s Server, uuid string) {
	_, _ = httpPost("http://"+hostPort(s.HostIP, s.Port)+"/speed/dovalid?key="+uuid, 5*time.Second)
}
