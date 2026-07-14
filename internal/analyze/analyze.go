// Package analyze는 수집된 세션·사용 데이터에서 비효율 발견을 산출한다.
// 규칙은 전부 결정적이다 — 토큰 절감 서비스가 분석에 LLM 토큰을 태우는 자기모순 금지(스펙 비목표).
package analyze

import (
	"fmt"
	"sort"

	"github.com/nohdol97/agent-coach/internal/agentsview"
	"github.com/nohdol97/agent-coach/internal/config"
)

// Finding 하나가 규칙 하나의 판정 결과다.
type Finding struct {
	Rule    string  // 규칙 식별자 (리포트 앵커)
	Score   float64 // 낭비 추정 가중치 — 관리 블록 상위 N 선별 기준
	Summary string  // 리포트용 상세 (근거 수치 포함)
	Advice  string  // 관리 블록용 한 줄 — 에이전트가 읽는 명령형 지침
}

type Result struct {
	SessionCount int
	TotalTokens  int64 // input+output+cache 4계열 합 — 추세 비교용
	TotalCost    float64
	// 추세(규칙 ①): 직전 구간 합계가 있을 때만 유효.
	HasTrend      bool
	TrendTokenPct float64
	TrendCostPct  float64
	Findings      []Finding // Score 내림차순
}

type Input struct {
	Cfg        config.Config
	Sessions   []agentsview.Session
	Usage      agentsview.Usage
	PrevTokens int64
	PrevCost   float64
}

func Run(in Input) Result {
	var r Result
	r.SessionCount = len(in.Sessions)
	for _, d := range in.Usage.Daily {
		r.TotalTokens += d.InputTokens + d.OutputTokens + d.CacheCreationTokens + d.CacheReadTokens
		r.TotalCost += d.TotalCost
	}
	if in.PrevTokens > 0 {
		r.HasTrend = true
		r.TrendTokenPct = pct(r.TotalTokens-in.PrevTokens, in.PrevTokens)
		if in.PrevCost > 0 {
			r.TrendCostPct = (r.TotalCost - in.PrevCost) / in.PrevCost * 100
		}
	}

	rules := []func(Input) *Finding{
		ruleHeavyContext,
		ruleCompaction,
		rulePromptHygiene,
		ruleRework,
		ruleModelMix,
	}
	for _, rule := range rules {
		if f := rule(in); f != nil {
			r.Findings = append(r.Findings, *f)
		}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool { return r.Findings[i].Score > r.Findings[j].Score })
	return r
}

// 규칙 ②: 대형 컨텍스트 — peak_context_tokens 임계 초과 세션.
// 컨텍스트가 클수록 캐시 재작성·재읽기 비용이 배수로 붙는다.
func ruleHeavyContext(in Input) *Finding {
	threshold := in.Cfg.PeakContextThreshold
	count := 0
	var worst int64
	for _, s := range in.Sessions {
		if s.PeakContextTokens > int64(threshold) {
			count++
			if s.PeakContextTokens > worst {
				worst = s.PeakContextTokens
			}
		}
	}
	if count == 0 {
		return nil
	}
	return &Finding{
		Rule:  "heavy-context",
		Score: float64(count) * 10,
		Summary: fmt.Sprintf("peak context %dk 초과 세션 %d건 (최대 %dk) — 임계 config.peak_context_threshold=%d",
			threshold/1000, count, worst/1000, threshold),
		Advice: fmt.Sprintf("컨텍스트 %dk 초과 세션이 %d건 — 파일은 필요한 부분만 읽고, 작업 단위가 바뀌면 새 세션을 시작하라.",
			threshold/1000, count),
	}
}

// 규칙 ③: 컴팩션 — 컨텍스트 압축 발생은 이미 한도까지 밀어붙였다는 후행 신호.
// 작업 중(mid-task) 압축은 맥락 유실로 재작업까지 유발하므로 가중치를 더 준다.
func ruleCompaction(in Input) *Finding {
	total, mid := 0, 0
	for _, s := range in.Sessions {
		total += s.Compactions
		mid += s.MidTaskCompaction
	}
	if total == 0 {
		return nil
	}
	return &Finding{
		Rule:    "compaction",
		Score:   float64(total)*5 + float64(mid)*10,
		Summary: fmt.Sprintf("컨텍스트 압축 %d회 (작업 중 압축 %d회)", total, mid),
		Advice: fmt.Sprintf("컨텍스트 압축이 %d회 발생 — 세션이 길어지기 전에 작업을 나누고, 대화가 무거워지면 요약 후 새 세션으로 이어가라.",
			total),
	}
}

// 규칙 ④: 프롬프트 위생 — agentsview quality_signals 집계.
// 모호한 첫 지시는 정정 왕복을 만들고, 왕복 한 번마다 전체 컨텍스트 재과금이 붙는다.
func rulePromptHygiene(in Input) *Finding {
	var short, dup, noverify, loop int
	for _, s := range in.Sessions {
		short += s.QualitySignals.ShortPrompt
		dup += s.QualitySignals.DuplicatePrompt
		noverify += s.QualitySignals.MissingVerification
		loop += s.QualitySignals.RunawayToolLoop
	}
	total := short + dup + noverify + loop
	if total == 0 {
		return nil
	}
	// 지배 신호 하나만 지침으로 — 블록 한 줄에 신호 4종을 다 담으면 아무것도 전달되지 않는다.
	advice := ""
	switch max4(short, dup, noverify, loop) {
	case short:
		advice = fmt.Sprintf("모호한 한 줄 프롬프트가 %d건 — 요구사항·완료 기준을 첫 프롬프트에 명시해 정정 왕복을 줄여라.", short)
	case dup:
		advice = fmt.Sprintf("같은 프롬프트 반복이 %d건 — 첫 지시에 맥락과 제약을 충분히 담아라.", dup)
	case noverify:
		advice = fmt.Sprintf("검증 기준 없는 작업 지시가 %d건 — 완료를 무엇으로 확인할지 지시에 포함하라.", noverify)
	default:
		advice = fmt.Sprintf("도구 호출 루프 폭주가 %d건 — 같은 명령이 반복 실패하면 접근을 바꿔라.", loop)
	}
	return &Finding{
		Rule:    "prompt-hygiene",
		Score:   float64(total) * 4,
		Summary: fmt.Sprintf("품질 신호 합계 %d (짧은 프롬프트 %d · 중복 %d · 검증기준 부재 %d · 도구 루프 %d)", total, short, dup, noverify, loop),
		Advice:  advice,
	}
}

// 규칙 ⑤: 실패·재작업 — 도구 실패·재시도·수정 반복(churn)·낙제 등급 세션.
func ruleRework(in Input) *Finding {
	var fail, retry, churn, df int
	for _, s := range in.Sessions {
		fail += s.ToolFailures
		retry += s.ToolRetries
		churn += s.EditChurn
		if s.HealthGrade == "D" || s.HealthGrade == "F" {
			df++
		}
	}
	score := float64(fail)*2 + float64(retry)*2 + float64(churn)*3 + float64(df)*10
	if score == 0 {
		return nil
	}
	return &Finding{
		Rule:    "rework",
		Score:   score,
		Summary: fmt.Sprintf("도구 실패 %d · 재시도 %d · 수정 반복 %d · 건강도 D/F 세션 %d건", fail, retry, churn, df),
		Advice:  "실패한 접근을 그대로 재시도하지 말고, 원인을 확인한 뒤 진행하라 — 재시도 한 번마다 컨텍스트 전체가 재과금된다.",
	}
}

// 규칙 ⑥: 모델 믹스 — 최상위 단가 모델이 비용 대부분을 점유하면서 소형 세션이 많은 경우.
func ruleModelMix(in Input) *Finding {
	if in.Usage.Models == nil || len(in.Usage.Daily) == 0 {
		return nil
	}
	// 사용된 모델 중 입력 단가가 가장 높은 모델을 "프리미엄"으로 본다.
	costByModel := map[string]float64{}
	var totalCost float64
	for _, d := range in.Usage.Daily {
		for _, mb := range d.ModelBreakdowns {
			costByModel[mb.ModelName] += mb.Cost
			totalCost += mb.Cost
		}
	}
	if totalCost <= 0 || len(costByModel) == 0 {
		return nil
	}
	premium, premiumRate := "", -1.0
	for name := range costByModel {
		if p, ok := in.Usage.Models[name]; ok && p.InputCostPerMTok > premiumRate {
			premium, premiumRate = name, p.InputCostPerMTok
		}
	}
	if premium == "" {
		return nil
	}
	share := costByModel[premium] / totalCost
	if share < in.Cfg.PremiumCostShareThreshold {
		return nil
	}
	small := 0
	for _, s := range in.Sessions {
		if s.UserMessageCount <= 2 {
			small++
		}
	}
	if small == 0 {
		return nil // 전부 대형 작업이면 프리미엄 점유가 정당하다
	}
	return &Finding{
		Rule:  "model-mix",
		Score: share * 50,
		Summary: fmt.Sprintf("비용의 %.0f%%가 최상위 모델(%s), 소형 세션(사용자 메시지 ≤2) %d건",
			share*100, premium, small),
		Advice: fmt.Sprintf("비용의 %.0f%%가 %s에 집중 — 조회·소형 작업은 하위 모델 세션으로 분리하라.",
			share*100, premium),
	}
}

func pct(delta, base int64) float64 { return float64(delta) / float64(base) * 100 }

func max4(a, b, c, d int) int {
	m := a
	for _, v := range []int{b, c, d} {
		if v > m {
			m = v
		}
	}
	return m
}
