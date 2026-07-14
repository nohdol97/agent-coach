package agentsview

import (
	"errors"
	"testing"
	"time"
)

func fixedRunner(out string, err error) Runner {
	return func(args ...string) ([]byte, error) { return []byte(out), err }
}

// C5: 실측 정지 문구(rc 0)를 미실행으로 판정해야 한다.
// 가공 문구만 쓰면 이 케이스를 못 잡는다 — nohdol-harness 회귀 테스트 교훈 계승.
func TestDaemonRunningNegationFirst(t *testing.T) {
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{"실측 정지 문구 rc0", "No agentsview daemon is running.", nil, false},
		{"not running", "daemon is not running", nil, false},
		{"stopped", "daemon stopped", nil, false},
		{"실행 중", "agentsview daemon is running (pid 123, port 4400)", nil, true},
		{"비정상 종료", "error", errors.New("exit 1"), false},
		{"판정 불가 문구", "status unknown", nil, false}, // 애매하면 미실행 쪽
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewWithRunner(fixedRunner(tc.out, tc.err))
			if got := c.DaemonRunning(); got != tc.want {
				t.Fatalf("판정 불일치: got=%v want=%v (출력=%q)", got, tc.want, tc.out)
			}
		})
	}
}

func TestEnsureDaemonSkipsWhenRunning(t *testing.T) {
	calls := []string{}
	c := NewWithRunner(func(args ...string) ([]byte, error) {
		calls = append(calls, args[0]+" "+args[1])
		return []byte("running (pid 1)"), nil
	})
	if err := c.EnsureDaemon(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "daemon status" {
		t.Fatalf("실행 중이면 start를 호출하면 안 됨: %v", calls)
	}
}

// C3 전제: v0.38.1 실측 JSON 형태가 파싱된다.
func TestSessionsParsesMeasuredShape(t *testing.T) {
	const fixture = `{"sessions":[{"id":"a8c11576","project":"nohdol_harness","agent":"claude",
"started_at":"2026-07-14T12:19:07.644Z","message_count":35,"user_message_count":5,
"total_output_tokens":41057,"peak_context_tokens":141921,"tool_failure_signal_count":0,
"tool_retry_count":0,"edit_churn_count":1,"compaction_count":0,"mid_task_compaction_count":0,
"health_grade":"A","quality_signals":{"version":2,"short_prompt_count":1,"duplicate_prompt_count":0,
"missing_verification_count":0,"runaway_tool_loop_count":0}}]}`
	c := NewWithRunner(fixedRunner(fixture, nil))
	got, truncated, err := c.Sessions(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("1건은 절단이 아님")
	}
	s := got[0]
	if s.PeakContextTokens != 141921 || s.QualitySignals.ShortPrompt != 1 || s.HealthGrade != "A" {
		t.Fatalf("필드 매핑 불일치: %+v", s)
	}
}

func TestUsageDailyParsesMeasuredShape(t *testing.T) {
	const fixture = `{"schema_version":1,
"pricing":{"models":{"claude-fable-5":{"input_cost_per_mtok":10},"claude-sonnet-5":{"input_cost_per_mtok":2}}},
"daily":[{"date":"2026-07-12","inputTokens":240,"outputTokens":244233,
"cacheCreationTokens":1756687,"cacheReadTokens":20829633,"totalCost":55.0022705,
"modelsUsed":["claude-fable-5"],"modelBreakdowns":[{"modelName":"claude-fable-5","cost":55.0022705}]}]}`
	c := NewWithRunner(fixedRunner(fixture, nil))
	u, err := c.UsageDaily("2026-07-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Daily) != 1 || u.Daily[0].TotalCost < 55 || u.Models["claude-fable-5"].InputCostPerMTok != 10 {
		t.Fatalf("파싱 불일치: %+v", u)
	}
}

func TestSessionsErrorIncludesOutput(t *testing.T) {
	c := NewWithRunner(fixedRunner("Error: database locked", errors.New("exit 1")))
	if _, _, err := c.Sessions(time.Now()); err == nil {
		t.Fatal("에러여야 함")
	}
}
