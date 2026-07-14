package app

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/nohdol97/agent-coach/internal/config"
	"github.com/nohdol97/agent-coach/internal/paths"
	"github.com/nohdol97/agent-coach/internal/schedule"
	"github.com/nohdol97/agent-coach/internal/writeback"
)

type InstallOptions struct {
	Time         string // "HH:MM" — 빈 값이면 설정(기본 09:30) 유지
	NoAgentsview bool   // agentsview 자동 설치 시도 생략
}

// Install은 ①agentsview 확보 ②OS 스케줄 등록 ③초기화 ④첫 분석을 수행한다.
// agentsview 설치 실패는 안내 후 계속한다(스펙 R7) — analyze가 fail-open이므로
// 스케줄을 먼저 걸어두면 나중에 설치해도 다음 날부터 동작한다.
func Install(opts InstallOptions, deps Deps) int {
	deps.fill()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(deps.Out, "경고: %v — 기본 설정으로 진행\n", err)
	}
	if opts.Time != "" {
		if _, _, err := schedule.ParseHHMM(opts.Time); err != nil {
			fmt.Fprintf(deps.Out, "오류: %v\n", err)
			return 2
		}
		cfg.ScheduleTime = opts.Time
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(deps.Out, "오류: 설정 저장 실패 — %v\n", err)
		return 1
	}
	fmt.Fprintf(deps.Out, "설정 저장: %s\n", tildify(paths.ConfigPath()))

	// ① agentsview 확보
	if !deps.Installed() && !opts.NoAgentsview {
		fmt.Fprintln(deps.Out, "agentsview 미설치 — 공식 스크립트로 설치를 시도합니다…")
		if err := installAgentsview(); err != nil {
			fmt.Fprintf(deps.Out, "자동 설치 실패: %v\n  수동 설치: https://github.com/kenn-io/agentsview/releases 에서 OS에 맞는 바이너리를 받아 PATH에 두세요.\n  설치 없이도 스케줄 등록은 계속합니다 — 설치 후 별도 조치는 필요 없습니다.\n", err)
		}
	}
	if deps.Installed() {
		if err := deps.client().EnsureDaemon(); err != nil {
			fmt.Fprintf(deps.Out, "경고: agentsview 데몬 기동 실패 — %v\n", err)
		} else {
			fmt.Fprintln(deps.Out, "agentsview 데몬 확인 완료")
		}
	}

	// ② OS 스케줄 등록
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(deps.Out, "오류: 실행 파일 경로 확인 실패 — %v\n", err)
		return 1
	}
	desc, err := schedule.Register(exe, cfg.ScheduleTime)
	if err != nil {
		fmt.Fprintf(deps.Out, "오류: 스케줄 등록 실패 — %v\n", err)
		return 1 // 스케줄이 없으면 이 서비스는 존재하지 않는 것과 같다 — 정직하게 실패
	}
	fmt.Fprintln(deps.Out, desc)

	// ③④ 첫 분석 — 설치 직후 결과가 보여야 "설치 후 아무 일도 안 일어나는 프로그램"이 안 된다.
	fmt.Fprintln(deps.Out, "\n첫 분석을 실행합니다…")
	return Analyze(AnalyzeOptions{}, deps)
}

// installAgentsview는 공식 설치 스크립트를 실행한다(harness-install 3단계와 동일 경로).
func installAgentsview() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-ExecutionPolicy", "ByPass", "-NoProfile", "-Command",
			"irm https://agentsview.io/install.ps1 | iex")
	default:
		cmd = exec.Command("bash", "-c", "curl -fsSL https://agentsview.io/install.sh | bash")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, lastLine(out))
	}
	return nil
}

type UninstallOptions struct {
	Purge bool // 데이터(~/.agentcoach)까지 삭제
}

// Uninstall은 스케줄 해제 + 관리 블록 제거를 수행한다(스펙 R7).
// 부분 실패는 경고로 전부 보고한다 — 절반 성공한 제거를 성공으로 위장하지 않는다.
func Uninstall(opts UninstallOptions, deps Deps) int {
	deps.fill()
	code := 0

	for _, w := range deps.Unregister() {
		fmt.Fprintf(deps.Out, "경고: %s\n", w)
		code = 1
	}
	fmt.Fprintln(deps.Out, "스케줄 해제 완료")

	cfg, _ := config.Load()
	for _, target := range cfg.Targets {
		removed, err := writeback.RemoveBlock(paths.Expand(target))
		if err != nil {
			fmt.Fprintf(deps.Out, "경고: %s 블록 제거 실패 — %v\n", target, err)
			code = 1
			continue
		}
		if removed {
			fmt.Fprintf(deps.Out, "관리 블록 제거: %s\n", target)
		}
	}

	if opts.Purge {
		if err := os.RemoveAll(paths.DataDir()); err != nil {
			fmt.Fprintf(deps.Out, "경고: 데이터 삭제 실패 — %v\n", err)
			code = 1
		} else {
			fmt.Fprintf(deps.Out, "데이터 삭제: %s\n", tildify(paths.DataDir()))
		}
	} else {
		fmt.Fprintf(deps.Out, "데이터 보존: %s (완전 삭제는 --purge)\n", tildify(paths.DataDir()))
	}
	return code
}

func lastLine(b []byte) string {
	s := string(b)
	lines := []string{}
	for _, l := range splitLines(s) {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	l := lines[len(lines)-1]
	if len(l) > 200 {
		l = l[:200]
	}
	return l
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
