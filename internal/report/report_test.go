package report

import (
	"strings"
	"testing"
	"time"

	"github.com/nohdol97/agent-coach/internal/analyze"
)

func TestRenderContainsTrendAndNotes(t *testing.T) {
	res := analyze.Result{
		SessionCount: 5, TotalTokens: 1234567, TotalCost: 42.5,
		HasTrend: true, TrendTokenPct: -18.2, TrendCostPct: -12.0,
		Findings: []analyze.Finding{{Rule: "heavy-context", Score: 30, Summary: "근거", Advice: "지침"}},
	}
	from := time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	md := Render("2026-07-14", from, to, res, []string{"세션 목록이 500건에서 절단됨"})

	for _, want := range []string{"-18.2%", "heavy-context", "절단됨", "1,234,567", "$42.50"} {
		if !strings.Contains(md, want) {
			t.Fatalf("%q 누락:\n%s", want, md)
		}
	}
}

func TestRenderFirstMeasurement(t *testing.T) {
	md := Render("2026-07-14", time.Now(), time.Now(), analyze.Result{}, nil)
	if !strings.Contains(md, "최초 측정") {
		t.Fatalf("최초 측정 문구 누락:\n%s", md)
	}
	if !strings.Contains(md, "주목할 비효율이 없었다") {
		t.Fatalf("무발견 문구 누락:\n%s", md)
	}
}

func TestSaveAndLatest(t *testing.T) {
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())
	if _, err := LatestPath(); err == nil {
		t.Fatal("리포트 없음이 에러여야 함")
	}
	if _, err := Save("2026-07-13", "old"); err != nil {
		t.Fatal(err)
	}
	p2, err := Save("2026-07-14", "new")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := LatestPath()
	if err != nil {
		t.Fatal(err)
	}
	if latest != p2 {
		t.Fatalf("최신 리포트 불일치: %s != %s", latest, p2)
	}
}
