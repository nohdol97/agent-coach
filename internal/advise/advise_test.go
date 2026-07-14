package advise

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nohdol97/agent-coach/internal/analyze"
)

func findings(n int) analyze.Result {
	var r analyze.Result
	for i := 0; i < n; i++ {
		r.Findings = append(r.Findings, analyze.Finding{
			Rule: fmt.Sprintf("r%d", i), Score: float64(n - i),
			Advice: fmt.Sprintf("지침 %d — 반복 낭비를 줄여라.", i),
		})
	}
	return r
}

// C2: 발견이 많아도 블록 전체가 15줄을 넘지 않는다.
func TestBlockCapAt15Lines(t *testing.T) {
	block := Block("2026-07-14", findings(30), 30, "~/.agentcoach/reports/2026-07-14.md")
	if len(block) > MaxBlockLines {
		t.Fatalf("블록 %d줄 — 상한 %d 초과", len(block), MaxBlockLines)
	}
	if block[0] != BeginMarker || block[len(block)-1] != EndMarker {
		t.Fatalf("마커 위치 불일치: %v", block)
	}
	// 리포트 경로 줄은 지침을 덜어내도 살아남아야 한다.
	if !strings.HasPrefix(block[len(block)-2], "전체 리포트:") {
		t.Fatalf("리포트 줄 유실: %v", block)
	}
}

func TestBlockTop3Default(t *testing.T) {
	block := Block("2026-07-14", findings(10), 3, "r.md")
	bullets := 0
	for _, l := range block {
		if strings.HasPrefix(l, "- ") {
			bullets++
		}
	}
	if bullets != 3 {
		t.Fatalf("지침 3개여야 함: %d개\n%s", bullets, strings.Join(block, "\n"))
	}
	// 점수 상위(첫 발견)가 포함돼야 한다.
	if !strings.Contains(strings.Join(block, "\n"), "지침 0") {
		t.Fatal("최상위 발견 누락")
	}
}

func TestBlockNoFindings(t *testing.T) {
	block := Block("2026-07-14", analyze.Result{}, 3, "r.md")
	joined := strings.Join(block, "\n")
	if !strings.Contains(joined, "비효율이 없었다") {
		t.Fatalf("무발견 문구 누락:\n%s", joined)
	}
}

func TestSanitizeSingleLine(t *testing.T) {
	r := analyze.Result{Findings: []analyze.Finding{{
		Advice: "줄바꿈이\n섞인   지침" + strings.Repeat(" 아주 긺", 100),
	}}}
	block := Block("2026-07-14", r, 3, "r.md")
	for _, l := range block {
		if strings.Contains(l, "\n") {
			t.Fatal("블록 줄에 줄바꿈이 남음")
		}
		if len([]rune(l)) > adviceMaxRune+2 {
			t.Fatalf("지침 길이 상한 초과: %d룬", len([]rune(l)))
		}
	}
}
