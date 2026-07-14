package writeback

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func block(advice string) []string {
	return []string{
		"<!-- agentcoach:begin v1 (자동 생성 — 직접 수정 금지, 매일 갱신) -->",
		"## 토큰 효율 지침 (agent-coach, 2026-07-14)",
		"- " + advice,
		"<!-- agentcoach:end -->",
	}
}

func setup(t *testing.T, initial string) string {
	t.Helper()
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	if initial != "" {
		if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// C1: 왕복 멱등 — 같은 내용 2회 쓰기는 파일 불변, 다른 내용은 블록만 교체, 블록 외 보존.
func TestUpsertIdempotentAndReplaces(t *testing.T) {
	userContent := "# 내 전역 지침\n\n- 내가 쓴 규칙 1\n"
	p := setup(t, userContent)

	changed, err := UpsertBlock(p, block("지침 A"))
	if err != nil || !changed {
		t.Fatalf("첫 기입 실패: changed=%v err=%v", changed, err)
	}
	after1, _ := os.ReadFile(p)

	changed, err = UpsertBlock(p, block("지침 A"))
	if err != nil || changed {
		t.Fatalf("같은 내용 재기입은 무변경이어야 함: changed=%v err=%v", changed, err)
	}
	after2, _ := os.ReadFile(p)
	if string(after1) != string(after2) {
		t.Fatal("멱등 위반 — 파일이 변함")
	}

	changed, err = UpsertBlock(p, block("지침 B"))
	if err != nil || !changed {
		t.Fatalf("갱신 실패: %v", err)
	}
	got := string(mustRead(t, p))
	if !strings.Contains(got, "지침 B") || strings.Contains(got, "지침 A") {
		t.Fatalf("블록 교체 실패:\n%s", got)
	}
	if !strings.Contains(got, "내가 쓴 규칙 1") {
		t.Fatalf("블록 외 사용자 내용 유실:\n%s", got)
	}
	if strings.Count(got, "agentcoach:begin") != 1 {
		t.Fatalf("블록이 중복됨:\n%s", got)
	}
}

// C1: 마커 훼손 3종 → 쓰기 포기 + 원본 불변.
func TestUpsertCorruptedMarkers(t *testing.T) {
	cases := map[string]string{
		"end 없음":  "본문\n<!-- agentcoach:begin v1 -->\n지침\n",
		"순서 역전":   "<!-- agentcoach:end -->\n<!-- agentcoach:begin v1 -->\n",
		"begin 중복": "<!-- agentcoach:begin v1 -->\nx\n<!-- agentcoach:end -->\n<!-- agentcoach:begin v1 -->\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			p := setup(t, content)
			_, err := UpsertBlock(p, block("새 지침"))
			if !errors.Is(err, ErrCorrupted) {
				t.Fatalf("ErrCorrupted여야 함: %v", err)
			}
			if string(mustRead(t, p)) != content {
				t.Fatal("훼손 시 원본이 변하면 안 됨")
			}
		})
	}
}

func TestUpsertSkipsWhenDirMissing(t *testing.T) {
	t.Setenv("AGENTCOACH_DATA_DIR", t.TempDir())
	p := filepath.Join(t.TempDir(), "없는디렉토리", "AGENTS.md")
	if _, err := UpsertBlock(p, block("x")); !errors.Is(err, ErrSkipped) {
		t.Fatalf("디렉토리 부재는 ErrSkipped여야 함: %v", err)
	}
}

func TestUpsertCreatesFileWhenDirExists(t *testing.T) {
	p := setup(t, "") // 파일 없음, 디렉토리는 있음
	changed, err := UpsertBlock(p, block("첫 지침"))
	if err != nil || !changed {
		t.Fatalf("파일 생성 실패: %v", err)
	}
	if !strings.Contains(string(mustRead(t, p)), "첫 지침") {
		t.Fatal("생성 내용 불일치")
	}
}

// C9: RemoveBlock이 블록만 제거하고 나머지를 보존한다.
func TestRemoveBlock(t *testing.T) {
	p := setup(t, "# 내 지침\n")
	if _, err := UpsertBlock(p, block("지침")); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveBlock(p)
	if err != nil || !removed {
		t.Fatalf("제거 실패: %v", err)
	}
	got := string(mustRead(t, p))
	if strings.Contains(got, "agentcoach") {
		t.Fatalf("블록 잔재:\n%s", got)
	}
	if !strings.Contains(got, "# 내 지침") {
		t.Fatalf("사용자 내용 유실:\n%s", got)
	}
	// 블록 없는 파일에 재호출 → 무동작
	removed, err = RemoveBlock(p)
	if err != nil || removed {
		t.Fatalf("무블록 제거는 무동작이어야 함: removed=%v err=%v", removed, err)
	}
}

// 백업이 생성되고 파일당 7개만 유지된다.
func TestBackupRotation(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTCOACH_DATA_DIR", dataDir)
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p, []byte("원본\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := UpsertBlock(p, block("지침 "+strings.Repeat("x", i+1))); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 7 {
		t.Fatalf("백업 %d개 — 7개 초과", len(entries))
	}
	if len(entries) == 0 {
		t.Fatal("백업이 없음")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
