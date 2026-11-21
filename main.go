package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	cyan   = "\033[36m"
	bold   = "\033[1m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[2m"
	nc     = "\033[0m"
)

func useColor() bool {
	st, _ := os.Stdout.Stat()
	return st.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
}

func c(code, s string) string {
	if !useColor() {
		return s
	}
	return code + s + nc
}

var isps = []string{"电信", "联通", "移动"}

var bsg = [][2]string{{"北京", "北京"}, {"上海", "上海"}, {"广东", "广州"}}

var provinceCity = map[string]string{
	"北京": "北京", "天津": "天津", "上海": "上海", "重庆": "重庆",
	"河北": "石家庄", "山西": "太原", "内蒙古": "呼和浩特",
	"辽宁": "沈阳", "吉林": "长春", "黑龙江": "哈尔滨",
	"江苏": "南京", "浙江": "杭州", "安徽": "合肥", "福建": "福州",
	"江西": "南昌", "山东": "济南", "河南": "郑州", "湖北": "武汉",
	"湖南": "长沙", "广东": "广州", "广西": "南宁", "海南": "海口",
	"四川": "成都", "贵州": "贵阳", "云南": "昆明", "西藏": "拉萨",
	"陕西": "西安", "甘肃": "兰州", "青海": "西宁", "宁夏": "银川",
	"新疆": "乌鲁木齐", "台湾": "台北", "香港": "香港", "澳门": "澳门",
}

var extraCity = map[string][2]string{
	"深圳": {"广东", "深圳"}, "苏州": {"江苏", "苏州"}, "宁波": {"浙江", "宁波"},
	"青岛": {"山东", "青岛"}, "厦门": {"福建", "厦门"}, "东莞": {"广东", "东莞"},
	"无锡": {"江苏", "无锡"}, "佛山": {"广东", "佛山"},
}

type ProbeResult struct {
	Region      string  `json:"region"`
	Family      string  `json:"family"`
	RTT         string  `json:"rtt"`
	SingleUp    string  `json:"single_up"`
	SingleDown  string  `json:"single_down"`
	MultiUp     string  `json:"multi_up"`
	MultiDown   string  `json:"multi_down"`
	SingleUpF   float64 `json:"single_up_f"`
	SingleDownF float64 `json:"single_down_f"`
	MultiUpF    float64 `json:"multi_up_f"`
	MultiDownF  float64 `json:"multi_down_f"`
	Retrans     string  `json:"retrans"`         // 回程（上传方向）TCP 重传率，优先取单线程
	RetransF    float64 `json:"retrans_f"`       // 百分比，-1 表示不可用
	Retried     int     `json:"retried"`         // 因失败自动重测的次数
	Err         string  `json:"error,omitempty"` // 没测到数据的原因
}

var debugMode bool

// debugf 在 --debug 时输出排错信息到 stderr（先清掉进度行）。
func debugf(format string, a ...any) {
	if !debugMode {
		return
	}
	fmt.Fprintf(os.Stderr, "\r\033[2K%s[debug] %s%s\n", dim, fmt.Sprintf(format, a...), nc)
}

func dw(s string) int {
	n := 0
	for _, r := range s {
		if r > 127 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func padLeft(w int, s string) string {
	p := w - dw(s)
	if p < 0 {
		p = 0
	}
	return strings.Repeat(" ", p) + s
}

func fmtSpeed(v float64) string {
	if v < 0.001 {
		return "failed"
	}
	return fmt.Sprintf("%.2fMbps", v)
}

func fmtMS(v float64) string {
	if v < 0 {
		return "-"
	}
	if v < 10 {
		return fmt.Sprintf("%.1fms", v)
	}
	return fmt.Sprintf("%.0fms", v)
}

const legendText = "去程 = 测速点→本机（下载）  回程 = 本机→测速点（上传）  回程重传 = 单线程回程 TCP 重传段 ÷ 发出数据段"

func fmtRetrans(pct float64) string {
	if pct < 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f%%", pct)
}

// retransLevel 与 TcpQuality 一致：≤10% 好，10–20% 一般，≥20% 差
func retransLevel(pct float64) int {
	switch {
	case pct <= 10:
		return 0
	case pct < 20:
		return 1
	}
	return 2
}

func retransCell(text string, pct float64, w int) string {
	if pct < 0 {
		return c(dim, padLeft(w, "-"))
	}
	return c([]string{green, yellow, red}[retransLevel(pct)], padLeft(w, text))
}

func speedColor(v float64) string {
	if v < 0 || v <= 20 {
		return red
	}
	if v <= 150 {
		return yellow
	}
	return green
}

func cell(text string, v float64, w int) string {
	if text == "-" || text == "" {
		return c(dim, padLeft(w, "-"))
	}
	col := red
	if text != "failed" {
		col = speedColor(v)
	}
	return c(col, padLeft(w, text))
}

func printHeader(family string, modes []string) {
	parts := []string{c(cyan, padLeft(12, family)), c(cyan, padLeft(8, "延迟")), c(cyan, padLeft(10, "回程重传"))}
	if contains(modes, "single") {
		parts = append(parts, c(cyan, padLeft(12, "单线程去程")), c(cyan, padLeft(12, "单线程回程")))
	}
	if contains(modes, "multi") {
		parts = append(parts, c(cyan, padLeft(12, "多线程去程")), c(cyan, padLeft(12, "多线程回程")))
	}
	fmt.Println("  " + strings.Join(parts, "  "))
}

func printRow(r ProbeResult, modes []string) {
	parts := []string{c(cyan, padLeft(12, r.Region)), padLeft(8, r.RTT), retransCell(r.Retrans, r.RetransF, 10)}
	if contains(modes, "single") {
		parts = append(parts, cell(r.SingleDown, r.SingleDownF, 12), cell(r.SingleUp, r.SingleUpF, 12))
	}
	if contains(modes, "multi") {
		parts = append(parts, cell(r.MultiDown, r.MultiDownF, 12), cell(r.MultiUp, r.MultiUpF, 12))
	}
	if r.Retried > 0 {
		parts = append(parts, c(dim, fmt.Sprintf("已自动重测 ×%d", r.Retried)))
	}
	if r.Err != "" {
		parts = append(parts, c(yellow, r.Err))
	}
	fmt.Println("  " + strings.Join(parts, "  "))
}

func normalizeProvince(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{"特别行政区", "维吾尔自治区", "壮族自治区", "回族自治区", "自治区", "省", "市"} {
		s = strings.TrimSuffix(s, suf)
	}
	alias := map[string]string{"bj": "北京", "sh": "上海", "gd": "广东", "gz": "广东", "hb": "湖北", "js": "江苏"}
	if v, ok := alias[strings.ToLower(s)]; ok {
		return v
	}
	return s
}

func resolveLocation(name string) (prov, city string, err error) {
	loc := normalizeProvince(name)
	if loc == "" {
		return "", "", fmt.Errorf("空的测速点")
	}
	if v, ok := extraCity[loc]; ok {
		return v[0], v[1], nil
	}
	if v, ok := provinceCity[loc]; ok {
		return loc, v, nil
	}
	for p, cty := range provinceCity {
		if cty == loc {
			return p, cty, nil
		}
	}
	return "", "", fmt.Errorf("不支持的测速点: %s", name)
}

func targetsBSG() [][3]string {
	var jobs [][3]string
	for _, rc := range bsg {
		for _, isp := range isps {
			jobs = append(jobs, [3]string{rc[0], rc[1], isp})
		}
	}
	return jobs
}

func parsePoints(text string) ([][3]string, error) {
	text = strings.TrimSpace(toSimplified(text))
	if text == "" {
		return targetsBSG(), nil
	}
	re := regexp.MustCompile(`[,，、;；]+`)
	parts := re.Split(text, -1)
	var jobs [][3]string
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		var isp string
		for _, name := range isps {
			if strings.HasSuffix(part, name) {
				isp = name
				part = strings.TrimSpace(strings.TrimSuffix(part, name))
				break
			}
		}
		prov, city, err := resolveLocation(part)
		if err != nil {
			return nil, err
		}
		if isp != "" {
			jobs = append(jobs, [3]string{prov, city, isp})
		} else {
			for _, name := range isps {
				jobs = append(jobs, [3]string{prov, city, name})
			}
		}
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("没有解析到测速点")
	}
	return jobs, nil
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func prompt(r *bufio.Reader, msg, def string) string {
	fmt.Print(msg)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func interactive(info ClientInfo, v6 bool, askFamily bool) (jobs [][3]string, modes []string, wantV4, wantV6 bool, err error) {
	r := bufio.NewReader(os.Stdin)
	loc := info.City + info.Oper
	if loc == info.Oper {
		loc = info.Province + info.Oper
	}
	fmt.Printf("%s  全球网测 Linux 客户端\n\n", c(bold+cyan, "TaierSpeedtest"))
	fmt.Printf("  出口    %s  %s\n", loc, info.IP)
	if v6 {
		fmt.Println("  IPv6    可通互联网")
	} else {
		fmt.Println("  IPv6    不可用")
	}
	fmt.Println()
	fmt.Println("  测速点  回车 = 北上广省会 × 电信/联通/移动")
	fmt.Println("          可填省份、城市，多个用逗号分隔（简繁体皆可）")
	fmt.Println("          例: 湖北")
	fmt.Println("          例: 北京,上海,浙江")
	fmt.Println("          例: 武汉电信,杭州联通")
	fmt.Println()
	raw := prompt(r, "  请输入测速点: ", "")
	jobs, err = parsePoints(raw)
	if err != nil {
		return nil, nil, false, false, err
	}
	fmt.Println()
	fmt.Println("  测速模式  回车 = 单线程 + 多线程对照")
	fmt.Println("    1) 单+多对照  (默认)")
	fmt.Println("    2) 只测单线程")
	fmt.Println("    3) 只测多线程")
	fmt.Println()
	ch := prompt(r, "  请选择 [1/2/3]: ", "1")
	switch ch {
	case "2":
		modes = []string{"single"}
	case "3":
		modes = []string{"multi"}
	default:
		modes = []string{"single", "multi"}
	}
	wantV4, wantV6 = true, v6
	if askFamily {
		fmt.Println()
		if v6 {
			fmt.Println("  IP 协议  回车 = IPv4 + IPv6 都测")
			fmt.Println("    1) IPv4 + IPv6  (默认)")
		} else {
			fmt.Println("  IP 协议  回车 = 只测 IPv4（本机 IPv6 不可用）")
			fmt.Println("    1) IPv4 + IPv6")
		}
		fmt.Println("    2) 只测 IPv4")
		fmt.Println("    3) 只测 IPv6")
		fmt.Println()
		def := "1"
		if !v6 {
			def = "2"
		}
		switch prompt(r, "  请选择 [1/2/3]: ", def) {
		case "1":
			wantV4, wantV6 = true, true
		case "3":
			wantV4, wantV6 = false, true
		default:
			wantV4, wantV6 = true, false
		}
		if wantV6 && !v6 {
			fmt.Println(c(yellow, "  [!] 本机 IPv6 似乎不通，IPv6 结果可能全部失败"))
		}
	}
	return jobs, modes, wantV4, wantV6, nil
}

// parseFamily 解析 IP 协议选择：4 / 6 / both
func parseFamily(s string) (v4, v6 bool, err error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "4", "v4", "ipv4":
		return true, false, nil
	case "6", "v6", "ipv6":
		return false, true, nil
	case "both", "all", "46", "4+6", "dual":
		return true, true, nil
	}
	return false, false, fmt.Errorf("无效的 IP 协议选择: %s（可用 4 / 6 / both）", s)
}

func makeIMEI() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "TS" + strings.ToUpper(hex.EncodeToString(b))
}

func applyMode(r *ProbeResult, mode string, up, down float64) {
	us, ds := fmtSpeed(up), fmtSpeed(down)
	if mode == "single" {
		r.SingleUp, r.SingleDown = us, ds
		r.SingleUpF, r.SingleDownF = up, down
		return
	}
	r.MultiUp, r.MultiDown = us, ds
	r.MultiUpF, r.MultiDownF = up, down
}

// modeResult 是某个模式（单线程或多线程）一次测速的结果。
type modeResult struct {
	up, down float64
	rs       RetransStat
}

// fails 返回去程/回程中失败（≈0）的项数，2 表示完全没测到。
func (m modeResult) fails() int {
	n := 0
	if m.up < 0.001 {
		n++
	}
	if m.down < 0.001 {
		n++
	}
	return n
}

// runMode 为一个模式单独排队取 key，测完立即释放。
// 同一把 key 连续跑 单下→单上→多下→多上 时，后面的多线程（尤其回程）常被服务器拒绝而显示 failed，
// 因此每个模式各取一把新 key。
func runMode(s Server, imei, mode string, lengthS, intervalMS, downTh, upTh int) (modeResult, error) {
	uuid, err := enqueue(s, imei, 200)
	if err != nil {
		return modeResult{}, err
	}
	defer dequeue(s, uuid)
	dth, uth := downTh, upTh
	if mode == "single" {
		dth, uth = 1, 1
	}
	var m modeResult
	m.down, _ = runPhase(s, uuid, true, dth, lengthS, intervalMS)
	m.up, m.rs = runPhase(s, uuid, false, uth, lengthS, intervalMS)
	debugf("%s：去程 %.2f / 回程 %.2f Mbps，回程重传 %s", modeName(mode), m.down, m.up, fmtRetrans(m.rs.Percent()))
	return m, nil
}

func modeName(mode string) string {
	if mode == "single" {
		return "单线程"
	}
	return "多线程"
}

var retryDelay = 3 * time.Second

func runOne(base, ip, prov, city, isp, imei string, lengthS, intervalMS int, modes []string, downTh, upTh int, ipv6 bool, retries int) ProbeResult {
	family := "IPv4"
	if ipv6 {
		family = "IPv6"
	}
	r := ProbeResult{Region: prov + isp, Family: family, RTT: "-", SingleUp: "-", SingleDown: "-", MultiUp: "-", MultiDown: "-", Retrans: "-", RetransF: -1}
	debugf("── %s %s%s（%s）", family, prov, isp, city)
	servers, err := matchServers(base, ip, prov, city, isp, ipv6)
	if err != nil {
		debugf("查询测速节点失败：%v", err)
		r.Err = "查询测速节点失败"
		return r
	}
	debugf("候选测速节点 %d 个", len(servers))
	s := pickServer(servers, prov, city, isp)
	if s == nil {
		r.Err = "找不到测速节点"
		return r
	}
	debugf("选用 %s（%s，%s）", s.HostName, hostPort(s.HostIP, s.Port), s.City)
	if ipv6 && !strings.Contains(s.HostIP, ":") {
		debugf("注意：IPv6 测速却分配到 IPv4 地址的节点")
	}
	r.RTT = fmtMS(measureLatency(s.HostIP, s.Port))
	debugf("延迟 %s", r.RTT)
	var singleRS, multiRS RetransStat
	for _, mode := range modes {
		// 有任一方向失败就重测，保留失败最少的那一次
		var best modeResult
		var lastErr error
		bestFails := 3 // 比任何结果都差：连 key 都拿不到
		for try := 0; try <= retries; try++ {
			if try > 0 {
				time.Sleep(retryDelay)
				r.Retried++
			}
			m, err := runMode(*s, imei, mode, lengthS, intervalMS, downTh, upTh)
			f := 3
			if err == nil {
				f = m.fails()
			} else {
				lastErr = err
				debugf("%s 第 %d 次：%v", modeName(mode), try+1, err)
			}
			if f < bestFails {
				best, bestFails = m, f
			}
			if bestFails == 0 {
				break
			}
		}
		if bestFails == 3 {
			if r.Err == "" && lastErr != nil {
				r.Err = lastErr.Error()
			}
			continue // 排不到队，保持 "-"
		}
		if mode == "single" {
			singleRS.Add(best.rs)
		} else {
			multiRS.Add(best.rs)
		}
		applyMode(&r, mode, best.up, best.down)
	}
	rs := singleRS
	if !rs.OK {
		rs = multiRS
	}
	r.RetransF = rs.Percent()
	r.Retrans = fmtRetrans(r.RetransF)
	return r
}

var version = "dev"

func main() {
	bsgF := flag.Bool("bsg", false, "北上广三网")
	points := flag.String("points", "", "测速点，逗号分隔")
	province := flag.String("province", "", "指定省份")
	duration := flag.Int("duration", 5, "每方向秒数")
	interval := flag.Int("interval", 500, "采样间隔毫秒")
	downTh := flag.Int("down-threads", 8, "多线程下行连接数")
	upTh := flag.Int("up-threads", 4, "多线程上行连接数")
	modeF := flag.String("mode", "", "both|single|multi，交互时可不填")
	jsonF := flag.Bool("json", false, "JSON 输出")
	ipF := flag.String("ip", "", "IP 协议: 4|6|both，默认 IPv4，IPv6 可用时自动加测")
	ipv6F := flag.Bool("ipv6", false, "强制加测 IPv6（等同 --ip both）")
	noIPv6 := flag.Bool("no-ipv6", false, "不测 IPv6（等同 --ip 4）")
	noImage := flag.Bool("no-image", false, "不出图")
	retries := flag.Int("retries", 1, "测速失败自动重测次数（0-3）")
	queueSec := flag.Int("queue-wait", 15, "排队取测速 key 最长等待秒数")
	flag.BoolVar(&debugMode, "debug", false, "输出排错信息（失败原因、选用节点、排队过程）")
	flag.Parse()
	if *duration < 5 {
		*duration = 5
	}
	if *duration > 13 {
		*duration = 13
	}
	if *queueSec < 1 {
		*queueSec = 1
	}
	queueWait = time.Duration(*queueSec) * time.Second
	if *retries < 0 {
		*retries = 0
	}
	if *retries > 3 {
		*retries = 3
	}

	base, info, err := fetchClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[X] %v%s\n", red, err, nc)
		os.Exit(1)
	}

	// 决定 IP 协议：--ip 优先，其次 --ipv6 / --no-ipv6，否则交互询问或自动探测
	var wantV4, wantV6 bool
	familySet := false
	if *ipF != "" {
		wantV4, wantV6, err = parseFamily(*ipF)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s[X] %v%s\n", red, err, nc)
			os.Exit(1)
		}
		familySet = true
	} else if *noIPv6 {
		wantV4, wantV6, familySet = true, false, true
	} else if *ipv6F {
		wantV4, wantV6, familySet = true, true, true
	}
	v6OK := false
	if !familySet || wantV6 {
		v6OK = hasIPv6Internet()
	}
	if wantV6 && !v6OK {
		fmt.Fprintf(os.Stderr, "%s[!] 本机 IPv6 似乎不通，IPv6 结果可能全部失败%s\n", yellow, nc)
	}

	var jobs [][3]string
	var modes []string
	if *points != "" {
		jobs, err = parsePoints(*points)
		modes = []string{"single", "multi"}
	} else if *province != "" {
		jobs, err = parsePoints(*province)
		modes = []string{"single", "multi"}
	} else if *bsgF || !isTTY() {
		jobs = targetsBSG()
		modes = []string{"single", "multi"}
	} else {
		var v4, v6 bool
		jobs, modes, v4, v6, err = interactive(info, v6OK, !familySet)
		if !familySet {
			wantV4, wantV6, familySet = v4, v6, true
		}
	}
	if !familySet {
		wantV4, wantV6 = true, v6OK
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[X] %v%s\n", red, err, nc)
		os.Exit(1)
	}
	if *modeF == "single" {
		modes = []string{"single"}
	} else if *modeF == "multi" {
		modes = []string{"multi"}
	} else if *modeF == "both" {
		modes = []string{"single", "multi"}
	}

	imei := makeIMEI()
	loc := info.City + info.Oper
	fmt.Println()
	fmt.Println(c(bold+cyan, "泰尔测速"))
	fmt.Printf("%s  出口 %s  %s%s\n", dim, info.IP, loc, nc)
	fmt.Println()

	var rows []ProbeResult
	total := 0
	if wantV4 {
		total += len(jobs)
	}
	if wantV6 {
		total += len(jobs)
	}
	total *= len(modes)
	done := 0

	runFamily := func(ipv6 bool, family string) {
		if !*jsonF {
			if len(rows) > 0 {
				fmt.Println()
			}
			printHeader(family, modes)
		}
		for _, j := range jobs {
			if !*jsonF {
				tag := ""
				if ipv6 {
					tag = "IPv6 "
				}
				fmt.Fprintf(os.Stderr, "\r%s  测速进度%s  %d/%d  %s%s%s   ", cyan, nc, done, total, tag, j[0], j[2])
			}
			row := runOne(base, info.IP, j[0], j[1], j[2], imei, *duration, *interval, modes, *downTh, *upTh, ipv6, *retries)
			rows = append(rows, row)
			done += len(modes)
			if !*jsonF {
				fmt.Fprint(os.Stderr, "\r\033[2K")
				printRow(row, modes)
			}
		}
	}

	if wantV4 {
		runFamily(false, "IPv4")
	}
	if wantV6 {
		runFamily(true, "IPv6")
	}

	if *jsonF {
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Println(c(dim, "  "+legendText))
	fmt.Println()
	if *noImage {
		return
	}
	url, err := publishReport(info, rows, modes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[!] 结果图失败: %v%s\n", yellow, err, nc)
		return
	}
	fmt.Printf("%s结果图%s  %s\n", cyan, nc, url)
}
