package analyze

import (
	"testing"

	"github.com/nohdol97/agent-coach/internal/agentsview"
	"github.com/nohdol97/agent-coach/internal/config"
)

func baseInput() Input {
	return Input{
		Cfg: config.Default(),
		Sessions: []agentsview.Session{
			{ID: "s1", PeakContextTokens: 180000, MessageCount: 40, UserMessageCount: 5,
				Compactions: 2, MidTaskCompaction: 1, HealthGrade: "A",
				QualitySignals: agentsview.QualitySignals{ShortPrompt: 3, DuplicatePrompt: 1}},
			{ID: "s2", PeakContextTokens: 90000, UserMessageCount: 1, HealthGrade: "F",
				ToolFailures: 4, ToolRetries: 2, EditChurn: 3},
		},
		Usage: agentsview.Usage{
			Daily: []agentsview.DailyUsage{{
				Date: "2026-07-13", InputTokens: 100, OutputTokens: 200000,
				CacheCreationTokens: 1000000, CacheReadTokens: 20000000, TotalCost: 50,
				ModelBreakdowns: []agentsview.ModelBreakdown{
					{ModelName: "claude-fable-5", Cost: 48},
					{ModelName: "claude-sonnet-5", Cost: 2},
				},
			}},
			Models: map[string]agentsview.ModelPricing{
				"claude-fable-5":  {InputCostPerMTok: 10},
				"claude-sonnet-5": {InputCostPerMTok: 2},
			},
		},
	}
}

// C3: 실측 형태 픽스처에서 규칙들이 기대 발견을 산출한다.
func TestRunProducesExpectedFindings(t *testing.T) {
	res := Run(baseInput())

	want := map[string]bool{
		"heavy-context":  true, // s1: 180k > 150k
		"compaction":     true, // s1: 2회(작업 중 1회)
		"prompt-hygiene": true, // short 3 + dup 1
		"rework":         true, // s2: 실패 4·재시도 2·churn 3·F등급
		"model-mix":      true, // fable 96% 점유 + 소형 세션 1건
	}
	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.Rule] = true
		if f.Advice == "" || f.Summary == "" || f.Score <= 0 {
			t.Fatalf("발견 필드 누락: %+v", f)
		}
	}
	for rule := range want {
		if !got[rule] {
			t.Fatalf("규칙 %s 발견 누락. got=%v", rule, got)
		}
	}

	// Score 내림차순 정렬 확인 (관리 블록 상위 N 선별의 전제)
	for i := 1; i < len(res.Findings); i++ {
		if res.Findings[i].Score > res.Findings[i-1].Score {
			t.Fatalf("정렬 위반: %v", res.Findings)
		}
	}
	if res.TotalTokens != 100+200000+1000000+20000000 {
		t.Fatalf("토큰 합계 불일치: %d", res.TotalTokens)
	}
}

func TestCleanWindowHasNoFindings(t *testing.T) {
	in := Input{Cfg: config.Default(), Sessions: []agentsview.Session{
		{ID: "ok", PeakContextTokens: 50000, UserMessageCount: 4, HealthGrade: "A"},
	}}
	res := Run(in)
	if len(res.Findings) != 0 {
		t.Fatalf("깨끗한 구간에 발견이 있으면 안 됨: %+v", res.Findings)
	}
}

// 규칙 ①: 직전 구간 대비 추세.
func TestTrend(t *testing.T) {
	in := baseInput()
	in.PrevTokens = res0(t, in).TotalTokens / 2 // 직전 구간의 2배 사용
	in.PrevCost = 25
	res := Run(in)
	if !res.HasTrend {
		t.Fatal("PrevTokens가 있으면 추세가 있어야 함")
	}
	if res.TrendTokenPct < 99 || res.TrendTokenPct > 101 {
		t.Fatalf("토큰 추세 %.1f%% (기대 ~100%%)", res.TrendTokenPct)
	}
	if res.TrendCostPct < 99 || res.TrendCostPct > 101 {
		t.Fatalf("비용 추세 %.1f%% (기대 ~100%%)", res.TrendCostPct)
	}
}

func TestModelMixSkipsWhenBalanced(t *testing.T) {
	in := baseInput()
	in.Usage.Daily[0].ModelBreakdowns = []agentsview.ModelBreakdown{
		{ModelName: "claude-fable-5", Cost: 25},
		{ModelName: "claude-sonnet-5", Cost: 25},
	}
	for _, f := range Run(in).Findings {
		if f.Rule == "model-mix" {
			t.Fatalf("점유율 50%%에서 model-mix가 나오면 안 됨(임계 %.0f%%)", in.Cfg.PremiumCostShareThreshold*100)
		}
	}
}

func res0(t *testing.T, in Input) Result {
	t.Helper()
	return Run(in)
}
