// Package app은 명령별 파이프라인을 결선한다 — state → agentsview → analyze
// → advise → writeback+report+notify → state 갱신.
//
// analyze 경로의 원칙은 fail-open이다(스펙 R1): 수집·기입의 부분 실패는
// 전부 리포트 주석(notes)으로 남기고 exit 0 한다. 스케줄러가 부르는 명령이
// 크래시하면 사용자는 그 사실조차 모른다.
package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nohdol97/agent-coach/internal/advise"
	"github.com/nohdol97/agent-coach/internal/agentsview"
	"github.com/nohdol97/agent-coach/internal/analyze"
	"github.com/nohdol97/agent-coach/internal/config"
	"github.com/nohdol97/agent-coach/internal/notify"
	"github.com/nohdol97/agent-coach/internal/paths"
	"github.com/nohdol97/agent-coach/internal/report"
	"github.com/nohdol97/agent-coach/internal/schedule"
	"github.com/nohdol97/agent-coach/internal/state"
	"github.com/nohdol97/agent-coach/internal/writeback"
)

// Deps는 테스트에서 치환하는 외부 접점이다. 0값이면 실제 구현을 쓴다.
type Deps struct {
	Out        io.Writer
	Installed  func() bool
	Runner     agentsview.Runner // agentsview 프로세스 실행
	Notify     func(title, body string)
	Now        func() time.Time
	Unregister func() []string // 스케줄 해제 — 테스트가 실제 스케줄러를 건드리지 않게 주입 지점 분리
}

func (d *Deps) fill() {
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Installed == nil {
		d.Installed = agentsview.Installed
	}
	if d.Notify == nil {
		d.Notify = notify.Send
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Unregister == nil {
		d.Unregister = schedule.Unregister
	}
}

func (d *Deps) client() *agentsview.Client {
	if d.Runner != nil {
		return agentsview.NewWithRunner(d.Runner)
	}
	return agentsview.New()
}

type AnalyzeOptions struct {
	Force  bool
	DryRun bool
}

const defaultLookback = 7 * 24 * time.Hour // 최초 실행(워터마크 부재) 시 분석 구간

// Analyze는 일일 분석 한 사이클을 수행한다.
func Analyze(opts AnalyzeOptions, deps Deps) int {
	deps.fill()
	now := deps.Now()
	today := now.Format("2006-01-02")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(deps.Out, "경고: %v — 기본 설정으로 진행\n", err)
	}
	st, _ := state.Load()

	// 하루 1회 멱등(스펙 R6·C7) — DAILY+ONLOGON 2중 발화를 무해하게 만드는 장치.
	// dry-run은 아무것도 바꾸지 않으므로 멱등 게이트를 지나 언제든 미리보기가 가능하다.
	if st.LastRunDate == today && !opts.Force && !opts.DryRun {
		fmt.Fprintf(deps.Out, "오늘(%s) 분석은 이미 완료 — 건너뜀 (--force로 재실행)\n", today)
		return 0
	}

	// 측정 구간은 완결된 일 단위 버킷으로 잡는다: [지난 실행 날짜, 오늘) — usage daily가
	// 일 단위 집계라, 시각 워터마크를 쓰면 경계일이 두 구간에 이중 산입돼 추세가 상시 편향된다.
	fromDate := st.LastRunDate
	if fromDate == "" || fromDate >= today {
		// 최초 실행은 기본 소급 구간, 같은 날 --force 재실행은 어제 하루를 재계산한다.
		fromDate = now.Add(-defaultLookback).Format("2006-01-02")
		if fromDate >= today {
			fromDate = now.Add(-24 * time.Hour).Format("2006-01-02")
		}
	}
	from, _ := time.ParseInLocation("2006-01-02", fromDate, now.Location())
	windowEnd, _ := time.ParseInLocation("2006-01-02", today, now.Location())

	var notes []string
	var res analyze.Result
	dataOK := false // 세션·사용량이 모두 수집됐을 때만 지침을 갱신한다 (S1 — 거짓 안심 방지)

	if !deps.Installed() {
		// C4: agentsview 부재도 리포트는 남긴다 — 다음 행동(설치)을 리포트가 안내한다.
		notes = append(notes,
			"agentsview 미설치 — 분석 데이터 없음. 설치: `curl -fsSL https://agentsview.io/install.sh | bash` (Windows: `irm https://agentsview.io/install.ps1 | iex`), 차단 시 https://github.com/kenn-io/agentsview/releases 바이너리",
			"설치 후 `agentcoach analyze --force`로 즉시 재분석 가능")
	} else {
		client := deps.client()
		if err := client.EnsureDaemon(); err != nil {
			notes = append(notes, fmt.Sprintf("데몬 기동 실패(수집은 온디맨드 동기화로 계속): %v", err))
		}
		sessions, truncated, sessErr := client.Sessions(from)
		if sessErr != nil {
			notes = append(notes, fmt.Sprintf("세션 수집 실패: %v", sessErr))
		}
		if truncated {
			notes = append(notes, fmt.Sprintf("세션 목록이 상한(%d건)에서 절단됨 — 통계가 실제보다 작을 수 있음", 500))
		}
		usage, usageErr := client.UsageDaily(fromDate)
		if usageErr != nil {
			notes = append(notes, fmt.Sprintf("사용량 수집 실패: %v", usageErr))
		}
		// 오늘(진행 중인 날)의 부분 버킷은 제외한다 — 내일 실행이 완결분으로 집계한다.
		usage.Daily = clipDays(usage.Daily, fromDate, today)
		dataOK = sessErr == nil && usageErr == nil
		res = analyze.Run(analyze.Input{
			Cfg: cfg, Sessions: sessions, Usage: usage,
			PrevTokens: st.PrevTokens, PrevCost: st.PrevCost,
		})
	}

	reportPath := report.PathFor(today)
	block := advise.Block(today, res, cfg.MaxAdvice, tildify(reportPath))

	if opts.DryRun {
		fmt.Fprintf(deps.Out, "[dry-run] 관리 블록 미리보기:\n%s\n\n[dry-run] 리포트 미리보기:\n%s",
			strings.Join(block, "\n"), report.Render(today, from, windowEnd, res, notes))
		return 0
	}

	if !dataOK {
		// 데이터가 불완전한 날은 지침을 갱신하지 않는다 — "비효율 없음"으로 갱신하면
		// 측정 실패가 거짓 안심으로 둔갑한다. 이전 지침이 여전히 최선의 코칭이다.
		notes = append(notes, "수집 불완전 — 관리 블록 미갱신(이전 지침 유지)")
	} else {
		for _, target := range cfg.Targets {
			p := paths.Expand(target)
			changed, err := writeback.UpsertBlock(p, block)
			switch {
			case errors.Is(err, writeback.ErrSkipped):
				notes = append(notes, fmt.Sprintf("%s: 건너뜀(해당 CLI 미사용)", target))
			case errors.Is(err, writeback.ErrCorrupted):
				notes = append(notes, fmt.Sprintf("%s: 관리 블록 마커 훼손 — 쓰기 포기(원본 보존). 블록을 수동 정리한 뒤 다음 분석에서 재기입된다", target))
			case err != nil:
				notes = append(notes, fmt.Sprintf("%s: 기입 실패 — %v", target, err))
			case changed:
				fmt.Fprintf(deps.Out, "지침 기입: %s\n", target)
			}
		}
	}

	md := report.Render(today, from, windowEnd, res, notes)
	if _, err := report.Save(today, md); err != nil {
		fmt.Fprintf(deps.Out, "리포트 저장 실패: %v\n", err)
		return 1 // 리포트조차 못 남기면 이 사이클은 증거가 없다 — 유일한 비정상 종료
	}

	if cfg.NotifyEnabled {
		deps.Notify("agent-coach", notifyBody(res, dataOK))
	}

	st.Schema = 1
	st.LastRunDate = today
	st.Watermark = now.Format(time.RFC3339)
	if dataOK {
		// 실패한 날의 0값으로 기준선을 덮으면 다음 추세가 무의미해진다 — 성공 측정만 기준선이 된다.
		st.PrevTokens = res.TotalTokens
		st.PrevCost = res.TotalCost
	}
	if err := st.Save(); err != nil {
		fmt.Fprintf(deps.Out, "경고: 상태 저장 실패(다음 실행이 같은 구간을 재분석할 수 있음): %v\n", err)
	}

	fmt.Fprintf(deps.Out, "분석 완료 — 발견 %d건, 리포트: %s\n", len(res.Findings), tildify(reportPath))
	return 0
}

func notifyBody(res analyze.Result, dataOK bool) string {
	if !dataOK {
		return "측정 데이터 수집 실패 — 리포트를 확인하세요 (agentcoach report)"
	}
	if res.HasTrend {
		return fmt.Sprintf("지난 측정 대비 토큰 %+.0f%% · 비용 %+.0f%% · 코칭 %d건 갱신", res.TrendTokenPct, res.TrendCostPct, min(len(res.Findings), 3))
	}
	return fmt.Sprintf("첫 측정 완료 — 코칭 %d건 기입. 내일부터 증감이 표시됩니다", min(len(res.Findings), 3))
}

// clipDays는 일별 사용에서 [fromDate, today) 구간의 완결 버킷만 남긴다 —
// 오늘의 부분 버킷과 구간 밖 데이터를 제외해 구간 합계가 겹치지 않게 한다.
func clipDays(daily []agentsview.DailyUsage, fromDate, today string) []agentsview.DailyUsage {
	var out []agentsview.DailyUsage
	for _, d := range daily {
		if d.Date >= fromDate && d.Date < today {
			out = append(out, d)
		}
	}
	return out
}

// tildify는 홈 경로를 ~로 줄인다 — 관리 블록·출력에 절대 경로 소음을 줄인다.
func tildify(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home) {
		return p
	}
	return "~" + p[len(home):]
}
