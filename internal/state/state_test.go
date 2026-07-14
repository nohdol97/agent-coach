package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())

	s, err := Load()
	if err != nil {
		t.Fatalf("빈 상태 Load 실패: %v", err)
	}
	if s.Schema != 1 || s.LastRunDate != "" {
		t.Fatalf("초기 상태가 기본값이 아님: %+v", s)
	}

	s.LastRunDate = "2026-07-14"
	s.Watermark = "2026-07-14T09:30:00Z"
	s.PrevTokens = 12345
	s.PrevCost = 6.78
	if err := s.Save(); err != nil {
		t.Fatalf("Save 실패: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("재로드 실패: %v", err)
	}
	if got != s {
		t.Fatalf("왕복 불일치: got=%+v want=%+v", got, s)
	}
}

func TestLoadCorruptedResets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTCOACH_DATA_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{훼손"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err != nil {
		t.Fatalf("훼손 state는 초기화돼야 함(fail-open), got err=%v", err)
	}
	if s.LastRunDate != "" || s.Watermark != "" {
		t.Fatalf("훼손 state가 초기화되지 않음: %+v", s)
	}
}

func TestWatermarkTimeFallback(t *testing.T) {
	fb := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if got := (State{}).WatermarkTime(fb); !got.Equal(fb) {
		t.Fatalf("빈 워터마크는 fallback이어야 함: %v", got)
	}
	s := State{Watermark: "2026-07-14T09:30:00Z"}
	want := time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC)
	if got := s.WatermarkTime(fb); !got.Equal(want) {
		t.Fatalf("워터마크 파싱 불일치: %v", got)
	}
}
