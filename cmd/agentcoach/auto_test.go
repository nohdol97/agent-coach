package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// C10: 자동 모드 분기 — install만 만드는 config.json이 설치 여부의 근거다.
func TestIsInstalledByConfigPresence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTCOACH_DATA_DIR", dir)
	if isInstalled() {
		t.Fatal("config.json 없이 설치됨 판정")
	}
	// analyze의 산출물(state.json·리포트)은 설치 판정에 영향이 없어야 한다.
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isInstalled() {
		t.Fatal("state.json만으로 설치됨 판정 — install 산출물(config.json)이 기준이어야 함")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isInstalled() {
		t.Fatal("config.json이 있는데 미설치 판정")
	}
}

// C10: 비대화형 stdin(파이프)에서는 Enter 대기 없이 즉시 반환한다 —
// 스케줄러·CI에서 멈춘 프로세스가 되면 안 된다. (블로킹되면 테스트 타임아웃으로 실패)
func TestPauseSkipsWhenNotInteractive(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close() // 아무것도 쓰지 않는다 — 대기했다면 영원히 블록

	done := make(chan struct{})
	go func() {
		pauseIfInteractive(r)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("비대화형 stdin에서 Enter를 대기함")
	}
}
