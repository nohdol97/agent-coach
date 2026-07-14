// Package state는 측정 워터마크를 영속한다 — "지난 측정 대비"의 기준점.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nohdol97/agent-coach/internal/paths"
)

type State struct {
	Schema int `json:"schema"`
	// LastRunDate는 로컬 날짜("2006-01-02"). analyze의 하루 1회 멱등 판정에 쓴다(스펙 R6).
	LastRunDate string `json:"last_run_date"`
	// Watermark는 지난 측정 시각(RFC3339). 이 이후의 세션만 다음 분석 대상이다.
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

// WatermarkTime은 워터마크를 시각으로 돌려준다. 비어 있거나 훼손이면 fallback을 쓴다.
func (s State) WatermarkTime(fallback time.Time) time.Time {
	t, err := time.Parse(time.RFC3339, s.Watermark)
	if err != nil {
		return fallback
	}
	return t
}
