package main

import (
	"strings"
	"testing"
)

func TestTraditionalPoints(t *testing.T) {
	cases := map[string][3]string{
		"廣東電信":     {"广东", "广州", "电信"},
		"瀋陽聯通":     {"辽宁", "沈阳", "联通"},
		"臺灣":       {"台湾", "台北", "电信"},
		"內蒙古移動":    {"内蒙古", "呼和浩特", "移动"},
		"新疆維吾爾自治區": {"新疆", "乌鲁木齐", "电信"},
		"廈門電信":     {"福建", "厦门", "电信"},
		"哈爾濱聯通":    {"黑龙江", "哈尔滨", "联通"},
		"寧夏回族自治區":  {"宁夏", "银川", "电信"},
	}
	for in, want := range cases {
		jobs, err := parsePoints(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if jobs[0] != want {
			t.Errorf("%s: got %v want %v", in, jobs[0], want)
		}
	}
	jobs, err := parsePoints("北京，廣東、臺北電信")
	if err != nil || len(jobs) != 7 {
		t.Fatalf("mixed: %v %v", len(jobs), err)
	}
}

// 每个省份/城市名的繁体写法都必须能解析
func TestAllTraditionalNamesResolve(t *testing.T) {
	inv := map[string]string{}
	for i := 0; i+1 < len(t2sPairs); i += 2 {
		inv[t2sPairs[i+1]] = t2sPairs[i]
	}
	toTrad := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if v, ok := inv[string(r)]; ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	names := []string{}
	for p, c := range provinceCity {
		names = append(names, p, c)
	}
	for c := range extraCity {
		names = append(names, c)
	}
	for _, n := range names {
		if _, err := parsePoints(toTrad(n)); err != nil {
			t.Errorf("%s (%s): %v", n, toTrad(n), err)
		}
	}
}
