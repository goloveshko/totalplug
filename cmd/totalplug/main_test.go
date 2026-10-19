package main

import (
	"bufio"
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
		{"version long", []string{"--version"}, cliOptions{showVersion: true}},
		{"version short", []string{"-v"}, cliOptions{showVersion: true}},
		{"history", []string{"--history"}, cliOptions{history: true}},
		{"rollback space", []string{"--rollback", "CudaLister"}, cliOptions{rollbackKey: "CudaLister"}},
		{"rollback equals", []string{"--rollback=CudaLister"}, cliOptions{rollbackKey: "CudaLister"}},
		{"rollback quoted name", []string{"--rollback", "Total Packer"}, cliOptions{rollbackKey: "Total Packer"}},
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
		{"--rollback"},
		{"--history", "--rollback", "CudaLister"},
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

func TestReadBlindAnswer(t *testing.T) {
	cases := map[string]rune{
		"y\n":     'y',
		"YES\n":   'y',
		"  y  \n": 'y',
		"a\n":     'a',
		"all\n":   'a',
		"n\n":     'n',
		"\n":      'n',
		"junk\n":  'n',
	}
	for input, want := range cases {
		got := readBlindAnswer(bufio.NewReader(strings.NewReader(input)), "")
		if got != want {
			t.Errorf("readBlindAnswer(%q) = %q, want %q", input, got, want)
		}
	}

	// EOF without input must default to skip.
	if got := readBlindAnswer(bufio.NewReader(strings.NewReader("")), ""); got != 'n' {
		t.Errorf("readBlindAnswer(EOF) = %q, want 'n'", got)
	}
}
