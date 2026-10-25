package backups

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestList(t *testing.T) {
	if _, ok, err := (Dir{}).List(context.Background()); ok || err != nil {
		t.Fatalf("no folder: ok=%v err=%v, want not configured", ok, err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "daily"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	for i := range 9 {
		p := filepath.Join(dir, "daily", "happyx-202610"+string(rune('1'+i))+".sql.gz")
		if err := os.WriteFile(p, make([]byte, 10+i), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, base, base.AddDate(0, 0, i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "daily", "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Dir{Path: dir}.List(context.Background())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(got) != Max || got[0].Size != 18 || !got[0].Modified.Equal(base.AddDate(0, 0, 8)) {
		t.Fatalf("want the %d newest backups, newest first: %+v", Max, got)
	}
}
