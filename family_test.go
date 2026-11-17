package main

import "testing"

func TestParseFamily(t *testing.T) {
	cases := map[string][2]bool{
		"4": {true, false}, "IPv4": {true, false},
		"6": {false, true}, "v6": {false, true},
		"both": {true, true}, "46": {true, true},
	}
	for in, want := range cases {
		v4, v6, err := parseFamily(in)
		if err != nil || v4 != want[0] || v6 != want[1] {
			t.Errorf("parseFamily(%q) = %v,%v,%v", in, v4, v6, err)
		}
	}
	if _, _, err := parseFamily("5"); err == nil {
		t.Error("expected error for invalid input")
	}
}
