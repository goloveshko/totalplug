package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goloveshko/totalplug/internal/domain"
)

func TestResolveIniExplicitPortable(t *testing.T) {
	dir := t.TempDir()
	iniPath := filepath.Join(dir, "wincmd.ini")
	if err := os.WriteFile(iniPath, []byte("[Configuration]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Portable layout: wincmd.exe sits next to the ini, so the TC dir is
	// derived from the ini location without touching the registry.
	if err := os.WriteFile(filepath.Join(dir, "wincmd.exe"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	gotIni, gotDir, err := ResolveIni(iniPath)
	if err != nil {
		t.Fatalf("ResolveIni: %v", err)
	}
	if gotIni != iniPath {
		t.Errorf("iniPath = %q, want %q", gotIni, iniPath)
	}
	if gotDir != dir {
		t.Errorf("tcDir = %q, want %q", gotDir, dir)
	}
}

func TestResolveIniRejectsMissingAndDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", "wincmd.ini")
	if _, _, err := ResolveIni(missing); err == nil {
		t.Error("expected error for missing ini, got nil")
	}

	dir := t.TempDir()
	if _, _, err := ResolveIni(dir); err == nil {
		t.Error("expected error when path is a directory, got nil")
	}
}

func TestResolveIniAutoHintMentionsFlag(t *testing.T) {
	// When no wincmd.ini exists anywhere the CLI-style error must still hint at
	// --ini. Only meaningful if auto-discovery genuinely fails on this host.
	iniPath, _, err := ResolveIni("")
	if err == nil {
		t.Skipf("host has a discoverable wincmd.ini at %s", iniPath)
	}
	if !strings.Contains(err.Error(), "--ini") {
		t.Errorf("auto-discovery error should mention --ini hint: %v", err)
	}
}

func TestBuildLegacyIDMapDropsAmbiguous(t *testing.T) {
	plugins := []domain.Plugin{
		{ID: "WCX:plugins/a/a.wcx", Type: domain.TypePacker, Name: "Shared"},
		{ID: "WCX:plugins/b/b.wcx", Type: domain.TypePacker, Name: "Shared"},
		{ID: "WLX:plugins/uniq/uniq.wlx", Type: domain.TypeLister, Name: "Unique"},
	}

	mapping := buildLegacyIDMap(plugins)
	if _, ok := mapping["WCX_Shared"]; ok {
		t.Error("ambiguous legacy ID must be dropped")
	}
	if got := mapping["WLX_Unique"]; got != "WLX:plugins/uniq/uniq.wlx" {
		t.Errorf("legacy mapping = %q, want stable new ID", got)
	}
}

func TestResolveIniExplicitSubfolderIni(t *testing.T) {
	// The layout a portable Total Commander actually uses: the ini one level down
	// in ini\, and on a 64-bit install no wincmd.exe at all - only totalcmd64.exe.
	// Resolving this to the ini's own folder expands every %COMMANDER_PATH% into
	// a directory that holds no plugins, which reads as an install with nothing in
	// it.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ini"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "totalcmd64.exe"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	iniPath := filepath.Join(dir, "ini", "wincmd.ini")
	if err := os.WriteFile(iniPath, []byte("[Configuration]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gotIni, gotDir, err := ResolveIni(iniPath)
	if err != nil {
		t.Fatalf("ResolveIni: %v", err)
	}
	if gotIni != iniPath {
		t.Errorf("iniPath = %q, want %q", gotIni, iniPath)
	}
	if gotDir != dir {
		t.Errorf("tcDir = %q, want %q", gotDir, dir)
	}
}
