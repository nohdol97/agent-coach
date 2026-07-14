// Package agentsview는 agentsview CLI(v0.38.1 실측)의 공개 표면만 감싼다.
// DB 스키마를 직접 읽지 않는 이유: agentsview는 모델 지식에 없는 2026년 도구라
// 내부 구조 가정이 위험하고, CLI의 --json 출력이 안정 계약이다(스펙 배경).
package agentsview

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner는 agentsview 프로세스 실행을 추상화한다 — 테스트에서 픽스처로 치환한다.
type Runner func(args ...string) ([]byte, error)

type Client struct {
	run Runner
}

func New() *Client {
	return &Client{run: func(args ...string) ([]byte, error) {
		cmd := exec.Command("agentsview", args...)
		return cmd.CombinedOutput()
	}}
}

func NewWithRunner(r Runner) *Client { return &Client{run: r} }

// Installed는 PATH에서 agentsview 바이너리를 찾는다.
func Installed() bool {
	_, err := exec.LookPath("agentsview")
	return err == nil
}

// DaemonRunning은 `daemon status` 출력을 부정문 우선 배제로 판정한다.
// 긍정 substring 단독 판정은 "No agentsview daemon is running."(rc 0, v0.38.1 실측)을
// 실행 중으로 오판해 재기동을 영원히 건너뛴다 — nohdol-harness 2026-07-14 장애 교훈.
// 애매하면 미실행으로 기운다: 중복 start(무해)가 죽은 데몬 방치보다 낫다.
func (c *Client) DaemonRunning() bool {
	out, err := c.run("daemon", "status")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(out))
	for _, neg := range []string{"not running", "no agentsview daemon is running", "stopped"} {
		if strings.Contains(s, neg) {
			return false
		}
	}
	return strings.Contains(s, "running")
}

// EnsureDaemon은 데몬이 없으면 기동을 시도한다. 실패해도 에러만 돌려주고
// 호출자는 계속 진행한다 — 데몬 없이도 CLI 질의는 온디맨드 동기화로 동작할 수 있다.
func (c *Client) EnsureDaemon() error {
	if c.DaemonRunning() {
		return nil
	}
	if _, err := c.run("daemon", "start"); err != nil {
		return fmt.Errorf("agentsview daemon start 실패: %w", err)
	}
	return nil
}

// Session은 `session list --json`의 한 항목 중 분석에 쓰는 필드만 담는다.
// 알 수 없는 필드는 무시된다 — 상위 버전 출력에 관대해야 한다.
type Session struct {
	ID                string         `json:"id"`
	Project           string         `json:"project"`
	Agent             string         `json:"agent"`
	StartedAt         time.Time      `json:"started_at"`
	MessageCount      int            `json:"message_count"`
	UserMessageCount  int            `json:"user_message_count"`
	TotalOutputTokens int64          `json:"total_output_tokens"`
	PeakContextTokens int64          `json:"peak_context_tokens"`
	ToolFailures      int            `json:"tool_failure_signal_count"`
	ToolRetries       int            `json:"tool_retry_count"`
	EditChurn         int            `json:"edit_churn_count"`
	Compactions       int            `json:"compaction_count"`
	MidTaskCompaction int            `json:"mid_task_compaction_count"`
	HealthGrade       string         `json:"health_grade"`
	QualitySignals    QualitySignals `json:"quality_signals"`
}

type QualitySignals struct {
	ShortPrompt         int `json:"short_prompt_count"`
	DuplicatePrompt     int `json:"duplicate_prompt_count"`
	MissingVerification int `json:"missing_verification_count"`
	RunawayToolLoop     int `json:"runaway_tool_loop_count"`
}

const sessionPageLimit = 500 // CLI 최대치 — 도달 시 절단 가능성을 호출자에 알린다

// Sessions는 워터마크 이후 활동한 세션을 돌려준다.
// truncated는 페이지 상한 도달(누락 가능) 신호다 — 조용한 절단 금지(리포트에 명시).
func (c *Client) Sessions(activeSince time.Time) (sessions []Session, truncated bool, err error) {
	out, err := c.run("session", "list", "--json",
		"--active-since", activeSince.UTC().Format(time.RFC3339),
		"--limit", fmt.Sprint(sessionPageLimit))
	if err != nil {
		return nil, false, fmt.Errorf("session list 실패: %w (%s)", err, firstLine(out))
	}
	var payload struct {
		Sessions []Session `json:"sessions"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, false, fmt.Errorf("session list JSON 파싱 실패: %w", err)
	}
	return payload.Sessions, len(payload.Sessions) >= sessionPageLimit, nil
}

// DailyUsage는 `usage daily --json`의 일별 합계다.
type DailyUsage struct {
	Date                string           `json:"date"`
	InputTokens         int64            `json:"inputTokens"`
	OutputTokens        int64            `json:"outputTokens"`
	CacheCreationTokens int64            `json:"cacheCreationTokens"`
	CacheReadTokens     int64            `json:"cacheReadTokens"`
	TotalCost           float64          `json:"totalCost"`
	ModelBreakdowns     []ModelBreakdown `json:"modelBreakdowns"`
}

type ModelBreakdown struct {
	ModelName string  `json:"modelName"`
	Cost      float64 `json:"cost"`
}

// ModelPricing은 모델 믹스 규칙에서 "최상위(비싼) 모델" 판별에 쓴다.
type ModelPricing struct {
	InputCostPerMTok float64 `json:"input_cost_per_mtok"`
}

type Usage struct {
	Daily  []DailyUsage
	Models map[string]ModelPricing
}

// UsageDaily는 since(로컬 날짜) 이후의 일별 사용을 돌려준다.
func (c *Client) UsageDaily(since string) (Usage, error) {
	out, err := c.run("usage", "daily", "--json", "--since", since)
	if err != nil {
		return Usage{}, fmt.Errorf("usage daily 실패: %w (%s)", err, firstLine(out))
	}
	var payload struct {
		Daily   []DailyUsage `json:"daily"`
		Pricing struct {
			Models map[string]ModelPricing `json:"models"`
		} `json:"pricing"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return Usage{}, fmt.Errorf("usage daily JSON 파싱 실패: %w", err)
	}
	return Usage{Daily: payload.Daily, Models: payload.Pricing.Models}, nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
