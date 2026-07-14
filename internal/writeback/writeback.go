// Package writeback은 사용자 전역 지침 파일(~/.claude/CLAUDE.md 등)에
// agentcoach 관리 블록만을 안전하게 넣고 뺀다.
//
// 사용자 파일은 사용자 소유다 — 이 패키지는 센티널 마커 사이만 만지고,
// 마커가 훼손돼 있으면 쓰기를 포기한다(fail-open, 스펙 R4). 원본보다 낫게
// 고치려는 시도가 원본을 부순다.
package writeback

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nohdol97/agent-coach/internal/paths"
)

const (
	beginPrefix = "<!-- agentcoach:begin"
	endPrefix   = "<!-- agentcoach:end"
	keepBackups = 7
)

// ErrSkipped: 대상 CLI 미사용(부모 디렉토리 부재) — 파일을 만들어주지 않는다.
// ~/.codex가 없는 사용자에게 디렉토리를 만들면 Codex 설치로 오인될 수 있다.
var ErrSkipped = errors.New("대상 디렉토리 부재 — 해당 CLI 미사용으로 판단해 건너뜀")

// ErrCorrupted: 마커 훼손(begin/end 개수 불일치·순서 역전) — 원본 불변 보장.
var ErrCorrupted = errors.New("관리 블록 마커 훼손 — 쓰기 포기(원본 보존)")

// UpsertBlock은 path의 관리 블록을 block(마커 포함 줄들)으로 교체한다.
// 블록이 없으면 말미에 추가하고, 내용이 같으면 아무것도 하지 않는다(멱등).
func UpsertBlock(path string, block []string) (changed bool, err error) {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return false, ErrSkipped
	}

	old, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return false, fmt.Errorf("읽기 실패: %w", readErr)
	}

	var newContent string
	if errors.Is(readErr, os.ErrNotExist) {
		newContent = strings.Join(block, "\n") + "\n"
	} else {
		lines := strings.Split(string(old), "\n")
		begin, end, markErr := findMarkers(lines)
		if markErr != nil {
			return false, markErr
		}
		var merged []string
		if begin < 0 { // 블록 없음 → 말미 추가 (빈 줄 하나로 분리)
			merged = trimTrailingEmpty(lines)
			if len(merged) > 0 {
				merged = append(merged, "")
			}
			merged = append(merged, block...)
		} else {
			merged = append(append(append([]string{}, lines[:begin]...), block...), lines[end+1:]...)
		}
		newContent = strings.Join(trimTrailingEmpty(merged), "\n") + "\n"
	}

	if string(old) == newContent {
		return false, nil
	}
	if len(old) > 0 {
		if err := backup(path, old); err != nil {
			return false, fmt.Errorf("백업 실패로 쓰기 중단: %w", err) // 백업 없이는 원본을 건드리지 않는다
		}
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("쓰기 실패: %w", err)
	}
	return true, nil
}

// RemoveBlock은 관리 블록을 제거한다(uninstall 경로, 스펙 C9). 블록이 없으면 무동작.
func RemoveBlock(path string) (removed bool, err error) {
	old, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return false, nil
		}
		return false, readErr
	}
	lines := strings.Split(string(old), "\n")
	begin, end, markErr := findMarkers(lines)
	if markErr != nil {
		return false, markErr
	}
	if begin < 0 {
		return false, nil
	}
	merged := append(append([]string{}, lines[:begin]...), lines[end+1:]...)
	merged = trimTrailingEmpty(merged)
	newContent := ""
	if len(merged) > 0 {
		newContent = strings.Join(merged, "\n") + "\n"
	}
	if err := backup(path, old); err != nil {
		return false, fmt.Errorf("백업 실패로 제거 중단: %w", err)
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// findMarkers는 유효한 블록 위치를 찾는다. 반환 (-1,-1,nil)은 "블록 없음".
func findMarkers(lines []string) (begin, end int, err error) {
	begin, end = -1, -1
	beginCount, endCount := 0, 0
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, beginPrefix) {
			beginCount++
			begin = i
		}
		if strings.HasPrefix(t, endPrefix) {
			endCount++
			end = i
		}
	}
	if beginCount == 0 && endCount == 0 {
		return -1, -1, nil
	}
	if beginCount != 1 || endCount != 1 || begin >= end {
		return -1, -1, ErrCorrupted
	}
	return begin, end, nil
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// backup은 원본을 ~/.agentcoach/backups/에 남기고 파일당 최근 7개만 유지한다.
func backup(path string, content []byte) error {
	dir := paths.BackupsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(path)
	name := fmt.Sprintf("%s.%s.bak", base, time.Now().Format("20060102-150405.000"))
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // 정리는 best-effort
	}
	var mine []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), base+".") && strings.HasSuffix(e.Name(), ".bak") {
			mine = append(mine, e.Name())
		}
	}
	sort.Strings(mine) // 타임스탬프 명명이라 사전순 == 시간순
	for len(mine) > keepBackups {
		_ = os.Remove(filepath.Join(dir, mine[0]))
		mine = mine[1:]
	}
	return nil
}
