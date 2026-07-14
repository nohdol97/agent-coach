package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nohdol97/agent-coach/internal/agentsview"
	"github.com/nohdol97/agent-coach/internal/state"
)

// 테스트 하네스: 데이터 디렉토리와 기입 대상을 임시 경로로 돌린 Deps를 만든다.
func harness(t *testing.T, installed bool, runner agentsview.Runner) (deps Deps, out *strings.Builder, targetFile string) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("AGENTCOACH_DATA_DIR", dataDir)

	targetDir := t.TempDir()
	targetFile = filepath.Join(targetDir, "CLAUDE.md")
	cfgJSON, _ := json.Marshal(map[string]any{
		"schema": 1, "targets": []string{targetFile},
		"notify_enabled": true, "max_advice": 3,
		"peak_context_threshold": 150000, "premium_cost_share_threshold": 0.85,
	})
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), cfgJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	out = &strings.Builder{}
	deps = Deps{
		Out:       out,
		Installed: func() bool { return installed },
		Runner:    runner,
		Notify:     func(string, string) {},
		Now:        func() time.Time { return time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC) },
		Unregister: func() []string { return nil },
	}
	return deps, out, targetFile
}

func fixtureRunner(t *testing.T) agentsview.Runner {
	t.Helper()
	return func(args ...string) ([]byte, error) {
		switch args[0] {
		case "daemon":
			return []byte("agentsview daemon is running (pid 1)"), nil
		case "session":
			return []byte(`{"sessions":[{"id":"s1","peak_context_tokens":180000,"user_message_count":5,
"compaction_count":1,"health_grade":"A","started_at":"2026-07-14T01:00:00Z",
"quality_signals":{"short_prompt_count":2}}]}`), nil
		case "usage":
			return []byte(`{"pricing":{"models":{"claude-fable-5":{"input_cost_per_mtok":10}}},
"daily":[{"date":"2026-07-13","outputTokens":1000,"totalCost":5,
"modelBreakdowns":[{"modelName":"claude-fable-5","cost":5}]}]}`), nil
		}
		t.Fatalf("예상 밖 호출: %v", args)
		return nil, nil
	}
}

// 정상 사이클: 기입·리포트·상태 갱신이 전부 일어난다.
func TestAnalyzeFullCycle(t *testing.T) {
	deps, out, target := harness(t, true, fixtureRunner(t))
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("관리 블록 미기입: %v", err)
	}
	if !strings.Contains(string(got), "agentcoach:begin") || !strings.Contains(string(got), "150k 초과") {
		t.Fatalf("블록 내용 불일치:\n%s", got)
	}

	st, _ := state.Load()
	if st.LastRunDate != "2026-07-14" || st.PrevTokens != 1000 || st.PrevCost != 5 {
		t.Fatalf("상태 갱신 불일치: %+v", st)
	}

	if _, err := os.Stat(filepath.Join(os.Getenv("AGENTCOACH_DATA_DIR"), "reports", "2026-07-14.md")); err != nil {
		t.Fatalf("리포트 미저장: %v", err)
	}
}

// C7: 같은 날 2회 → 스킵, --force → 실행.
func TestAnalyzeDailyIdempotent(t *testing.T) {
	deps, out, _ := harness(t, true, fixtureRunner(t))
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatal(out.String())
	}
	out.Reset()
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), "건너뜀") {
		t.Fatalf("같은 날 재실행이 스킵되지 않음:\n%s", out.String())
	}
	out.Reset()
	if code := Analyze(AnalyzeOptions{Force: true}, deps); code != 0 {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), "건너뜀") {
		t.Fatalf("--force가 스킵됨:\n%s", out.String())
	}
}

// C4: agentsview 부재 → exit 0 + 리포트에 미설치 안내.
func TestAnalyzeWithoutAgentsview(t *testing.T) {
	deps, out, _ := harness(t, false, nil)
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatalf("미설치는 exit 0이어야 함(fail-open): %d\n%s", code, out.String())
	}
	md, err := os.ReadFile(filepath.Join(os.Getenv("AGENTCOACH_DATA_DIR"), "reports", "2026-07-14.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "agentsview 미설치") {
		t.Fatalf("미설치 안내 누락:\n%s", md)
	}
}

// dry-run은 어떤 파일도 만들지 않는다.
func TestAnalyzeDryRun(t *testing.T) {
	deps, out, target := harness(t, true, fixtureRunner(t))
	if code := Analyze(AnalyzeOptions{DryRun: true}, deps); code != 0 {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("dry-run이 대상 파일을 만듦")
	}
	st, _ := state.Load()
	if st.LastRunDate != "" {
		t.Fatal("dry-run이 상태를 갱신함")
	}
	if !strings.Contains(out.String(), "agentcoach:begin") {
		t.Fatal("미리보기 출력 누락")
	}
}

// 수집 실패(CLI 에러)도 fail-open — 리포트에 사유가 남는다.
func TestAnalyzeCollectorFailure(t *testing.T) {
	failing := func(args ...string) ([]byte, error) {
		if args[0] == "daemon" {
			return []byte("running"), nil
		}
		return []byte("Error: database locked"), os.ErrPermission
	}
	deps, out, _ := harness(t, true, failing)
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatalf("수집 실패도 exit 0이어야 함: %d\n%s", code, out.String())
	}
	md, _ := os.ReadFile(filepath.Join(os.Getenv("AGENTCOACH_DATA_DIR"), "reports", "2026-07-14.md"))
	if !strings.Contains(string(md), "수집 실패") {
		t.Fatalf("실패 사유 미기록:\n%s", md)
	}
}

// C9 경로: Uninstall이 관리 블록을 제거한다 (스케줄 해제는 주입 스텁 — 실제 스케줄러 불가침).
func TestUninstallRemovesBlocks(t *testing.T) {
	deps, out, target := harness(t, true, fixtureRunner(t))
	if code := Analyze(AnalyzeOptions{}, deps); code != 0 {
		t.Fatal(out.String())
	}
	if code := Uninstall(UninstallOptions{}, deps); code != 0 {
		t.Fatalf("uninstall exit %d\n%s", code, out.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "agentcoach") {
		t.Fatalf("블록 잔재:\n%s", got)
	}
}
