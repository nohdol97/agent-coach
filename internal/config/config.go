// Package config는 ~/.agentcoach/config.json을 읽고 쓴다.
// 형식이 JSON인 이유: 표준 라이브러리만으로 처리해 외부 의존 0을 유지한다(스펙 R9).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nohdol97/agent-coach/internal/paths"
)

type Config struct {
	Schema int `json:"schema"`
	// Targets는 관리 블록을 기입할 파일들. "~/" 표기를 유지하고 사용 시점에 확장한다.
	Targets []string `json:"targets"`
	// PeakContextThreshold를 넘는 peak_context_tokens 세션을 대형 컨텍스트로 판정한다.
	PeakContextThreshold int `json:"peak_context_threshold"`
	// PremiumCostShareThreshold 이상을 최상위 모델이 점유하면 모델 믹스 발견을 낸다.
	PremiumCostShareThreshold float64 `json:"premium_cost_share_threshold"`
	NotifyEnabled             bool    `json:"notify_enabled"`
	// ScheduleTime은 "HH:MM" — install이 OS 스케줄러에 등록하는 일일 실행 시각.
	ScheduleTime string `json:"schedule_time"`
	// MaxAdvice는 관리 블록에 들어가는 지침 최대 개수(블록 15줄 상한의 구성 요소).
	MaxAdvice int `json:"max_advice"`
}

func Default() Config {
	return Config{
		Schema:                    1,
		Targets:                   []string{"~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"},
		PeakContextThreshold:      150000,
		PremiumCostShareThreshold: 0.85,
		NotifyEnabled:             true,
		ScheduleTime:              "09:30",
		MaxAdvice:                 3,
	}
}

// Load는 설정 파일이 없으면 기본값을 돌려준다(설치 전 어떤 명령도 동작해야 한다 — fail-open).
// 파일이 있으면 기본값 위에 덮어 읽어, 구버전 설정에 없는 필드가 기본값을 유지하게 한다.
func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(paths.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return c, fmt.Errorf("config 읽기 실패: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), fmt.Errorf("config 파싱 실패(%s): %w", paths.ConfigPath(), err)
	}
	return c, nil
}

func (c Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigPath(), append(b, '\n'), 0o644)
}
