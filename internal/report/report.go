// Package report는 일일 분석 전체를 로컬 마크다운으로 남긴다.
// 관리 블록은 15줄 요약이고, 근거·상세는 전부 여기 있다(스펙 R5).
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nohdol97/agent-coach/internal/analyze"
	"github.com/nohdol97/agent-coach/internal/paths"
)

// Render는 리포트 마크다운 전문을 만든다. notes에는 커버리지 경고
// (세션 목록 절단, 건너뛴 대상 파일, agentsview 부재 등)를 싣는다 — 조용한 절단 금지.
func Render(date string, from, to time.Time, res analyze.Result, notes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# agent-coach 리포트 — %s\n\n", date)
	fmt.Fprintf(&b, "- 측정 구간: %s ~ %s\n", from.Local().Format("2006-01-02 15:04"), to.Local().Format("2006-01-02 15:04"))
	if res.HasTrend {
		fmt.Fprintf(&b, "- **지난 측정 대비: 토큰 %+.1f%% · 비용 %+.1f%%**\n", res.TrendTokenPct, res.TrendCostPct)
	} else {
		b.WriteString("- 지난 측정 대비: 최초 측정 — 다음 리포트부터 증감이 표시된다\n")
	}
	fmt.Fprintf(&b, "- 합계: 세션 %d건 · 토큰 %s · 비용 $%.2f\n\n", res.SessionCount, comma(res.TotalTokens), res.TotalCost)

	b.WriteString("## 발견\n\n")
	if len(res.Findings) == 0 {
		b.WriteString("주목할 비효율이 없었다.\n")
	}
	for i, f := range res.Findings {
		fmt.Fprintf(&b, "### %d. %s (점수 %.0f)\n\n- 근거: %s\n- 지침: %s\n\n", i+1, f.Rule, f.Score, f.Summary, f.Advice)
	}

	if len(notes) > 0 {
		b.WriteString("## 주석 (커버리지·건너뜀)\n\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}

func PathFor(date string) string { return filepath.Join(paths.ReportsDir(), date+".md") }

func Save(date, content string) (string, error) {
	if err := os.MkdirAll(paths.ReportsDir(), 0o755); err != nil {
		return "", err
	}
	p := PathFor(date)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// LatestPath는 가장 최근 리포트 경로를 돌려준다(YYYY-MM-DD.md 명명이라 사전순 == 시간순).
func LatestPath() (string, error) {
	entries, err := os.ReadDir(paths.ReportsDir())
	if err != nil {
		return "", fmt.Errorf("리포트 디렉토리 없음 — 아직 analyze가 실행되지 않았다: %w", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("리포트 없음 — `agentcoach analyze`를 먼저 실행하라")
	}
	sort.Strings(names)
	return filepath.Join(paths.ReportsDir(), names[len(names)-1]), nil
}

func comma(n int64) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
