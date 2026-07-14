package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefault(t *testing.T) {
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("설정 부재 시 기본값이어야 함: %v", err)
	}
	d := Default()
	if c.PeakContextThreshold != d.PeakContextThreshold || len(c.Targets) != 2 {
		t.Fatalf("기본값 불일치: %+v", c)
	}
}

func TestPartialConfigKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTCOACH_DATA_DIR", dir)
	// 구버전 설정에 없는 필드는 기본값을 유지해야 한다.
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"schema":1,"peak_context_threshold":99000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PeakContextThreshold != 99000 {
		t.Fatalf("명시 필드가 반영 안 됨: %d", c.PeakContextThreshold)
	}
	if c.MaxAdvice != Default().MaxAdvice || len(c.Targets) != 2 {
		t.Fatalf("누락 필드가 기본값이 아님: %+v", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())
	c := Default()
	c.ScheduleTime = "07:15"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ScheduleTime != "07:15" {
		t.Fatalf("왕복 불일치: %+v", got)
	}
}
