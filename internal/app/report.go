package app

import (
	"fmt"
	"os"

	"github.com/nohdol97/agent-coach/internal/report"
)

type ReportOptions struct {
	Date string // "YYYY-MM-DD" — 빈 값이면 최신
}

// Report는 저장된 리포트를 stdout으로 출력한다.
func Report(opts ReportOptions, deps Deps) int {
	deps.fill()
	path := ""
	if opts.Date != "" {
		path = report.PathFor(opts.Date)
	} else {
		p, err := report.LatestPath()
		if err != nil {
			fmt.Fprintf(deps.Out, "%v\n", err)
			return 1
		}
		path = p
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(deps.Out, "리포트 없음: %s\n", tildify(path))
		return 1
	}
	fmt.Fprint(deps.Out, string(b))
	return 0
}
