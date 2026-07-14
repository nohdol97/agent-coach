// Package advise는 분석 발견을 관리 블록 텍스트로 렌더링한다.
//
// 핵심 제약: 블록 전체 15줄 상한(스펙 R4). 전역 지침은 모든 세션에 로드되므로
// 긴 코칭은 그 자체가 토큰 낭비다 — 코칭 서비스의 자기모순 금지.
package advise

import (
	"fmt"
	"strings"

	"github.com/nohdol97/agent-coach/internal/analyze"
)

const (
	BeginMarker = "<!-- agentcoach:begin v1 (자동 생성 — 직접 수정 금지, 매일 갱신) -->"
	EndMarker   = "<!-- agentcoach:end -->"
	// MaxBlockLines는 마커 포함 블록 전체 줄 수 상한이다.
	MaxBlockLines = 15
	adviceMaxRune = 160 // 지침 한 줄 길이 상한 — 넘치면 자른다
)

// Block은 관리 블록 전체(마커 포함)를 줄 단위로 돌려준다.
func Block(date string, res analyze.Result, maxAdvice int, reportPath string) []string {
	if maxAdvice < 1 {
		maxAdvice = 1
	}
	lines := []string{
		BeginMarker,
		fmt.Sprintf("## 토큰 효율 지침 (agent-coach, %s)", date),
	}
	if len(res.Findings) == 0 {
		lines = append(lines, "- 지난 구간에서 주목할 비효율이 없었다 — 현재 사용 패턴을 유지하라.")
	}
	for i, f := range res.Findings {
		if i >= maxAdvice {
			break
		}
		lines = append(lines, "- "+sanitize(f.Advice))
	}
	lines = append(lines, "전체 리포트: "+reportPath, EndMarker)

	// 방어적 상한 — maxAdvice 설정이 커도 블록이 상한을 넘지 않게 뒤에서 지침을 덜어낸다.
	for len(lines) > MaxBlockLines {
		lines = append(lines[:len(lines)-3], lines[len(lines)-2:]...)
	}
	return lines
}

// sanitize는 지침을 반드시 한 줄로 유지한다 — 줄바꿈이 섞이면 블록 파싱과 줄 수 상한이 깨진다.
func sanitize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > adviceMaxRune {
		s = string(r[:adviceMaxRune-1]) + "…"
	}
	return s
}
