// Package state는 측정 워터마크를 영속한다 — "지난 측정 대비"의 기준점.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nohdol97/agent-coach/internal/paths"
)

type State struct {
	Schema int `json:"schema"`
	// LastRunDate는 로컬 날짜("2006-01-02"). 하루 1회 멱등 판정(스펙 R6)과
	// 측정 구간의 시작일(완결된 일 단위 버킷 [LastRunDate, 오늘))을 겸한다.
	LastRunDate string `json:"last_run_date"`
	// Watermark는 지난 실행 시각(RFC3339) — 진단·기록용.
	Watermark string `json:"watermark"`
	// 직전 구간 합계 — 추세(규칙 ①) 계산용.
	PrevTokens int64   `json:"prev_tokens"`
	PrevCost   float64 `json:"prev_cost"`
}

func Load() (State, error) {
	var s State
	b, err := os.ReadFile(paths.StatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Schema: 1}, nil
		}
		return s, fmt.Errorf("state 읽기 실패: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		// 훼손된 state는 초기화한다 — 워터마크 유실은 한 구간의 중복 분석일 뿐, 크래시보다 낫다.
		return State{Schema: 1}, nil
	}
	return s, nil
}

func (s State) Save() error {
	if err := os.MkdirAll(filepath.Dir(paths.StatePath()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// 세션 통계가 담기므로 소유자 전용 권한.
	return os.WriteFile(paths.StatePath(), append(b, '\n'), 0o600)
}

