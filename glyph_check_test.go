package main

import (
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestGlyphsPresent(t *testing.T) {
	f, err := sfnt.Parse(cjkFontBytes)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	text := legendText + "节点延迟单线程去程回程多线程回程重传0123456789.%-msMbpsfailed IPv46测速TaierSpeedtest全球网测报告时间：出口未知"
	text += "北京上海广东电信联通移动"
	for _, p := range provinceCity {
		text += p
	}
	for k := range provinceCity {
		text += k
	}
	missing := map[rune]bool{}
	for _, r := range text {
		if i, _ := f.GlyphIndex(&buf, r); i == 0 && r != ' ' {
			missing[r] = true
		}
	}
	var s string
	for r := range missing {
		s += string(r)
	}
	if s != "" {
		t.Fatalf("字体缺字: %s", s)
	}
}
