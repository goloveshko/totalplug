package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliOptions
	}{
		{"defaults", nil, cliOptions{}},
		{"refresh long", []string{"--refresh-cache"}, cliOptions{forceRefresh: true}},
		{"refresh short", []string{"-r"}, cliOptions{forceRefresh: true}},
		{"ini space", []string{"-c", `D:\TC\wincmd.ini`}, cliOptions{iniPath: `D:\TC\wincmd.ini`}},
		{"ini equals", []string{"--ini=D:\\TC\\wincmd.ini"}, cliOptions{iniPath: `D:\TC\wincmd.ini`}},
		{"update short", []string{"-u"}, cliOptions{update: true}},
		{"update long", []string{"--update"}, cliOptions{update: true}},
		{"update all", []string{"--update-all"}, cliOptions{updateAll: true}},
		{"combined", []string{"-r", "--update", "--ini", "x.ini"}, cliOptions{forceRefresh: true, update: true, iniPath: "x.ini"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseArgsRejectsBadInput(t *testing.T) {
	cases := [][]string{
		{"--unknown"},
		{"--ini"},
		{"-c"},
		{"--update", "-x"},
	}
	for _, args := range cases {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) expected error, got nil", args)
		}
	}
}

func TestConfirm(t *testing.T) {
	yes := []string{"y\n", "Y\n", "yes\n", "  YES  \n"}
	for _, in := range yes {
		if !confirm(strings.NewReader(in), "") {
			t.Errorf("confirm(%q) = false, want true", in)
		}
	}

	no := []string{"n\n", "\n", "maybe\n", ""}
	for _, in := range no {
		if confirm(strings.NewReader(in), "") {
			t.Errorf("confirm(%q) = true, want false", in)
		}
	}
}

func TestTruncateString(t *testing.T) {
	if got := truncateString("short", 10); got != "short" {
		t.Errorf("truncateString(short, 10) = %q", got)
	}
	if got := truncateString("abcdefghij", 8); got != "abcde..." {
		t.Errorf("truncateString(abcdefghij, 8) = %q", got)
	}
	if got := truncateString("abcdef", 2); got != "ab" {
		t.Errorf("truncateString(abcdef, 2) = %q", got)
	}
}
