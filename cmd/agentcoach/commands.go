package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nohdol97/agent-coach/internal/app"
)

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	timeFlag := fs.String("time", "", "일일 실행 시각 HH:MM (기본 09:30)")
	noAV := fs.Bool("no-agentsview", false, "agentsview 자동 설치 시도 생략")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return app.Install(app.InstallOptions{Time: *timeFlag, NoAgentsview: *noAV}, app.Deps{})
}

func cmdAnalyze(args []string) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	force := fs.Bool("force", false, "오늘 이미 실행했어도 재분석")
	dryRun := fs.Bool("dry-run", false, "파일을 건드리지 않고 블록·리포트 미리보기")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return app.Analyze(app.AnalyzeOptions{Force: *force, DryRun: *dryRun}, app.Deps{})
}

func cmdReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	date := fs.String("date", "", "리포트 날짜 YYYY-MM-DD (기본: 최신)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return app.Report(app.ReportOptions{Date: *date}, app.Deps{})
}

func cmdUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purge := fs.Bool("purge", false, "데이터(~/.agentcoach)까지 삭제")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *purge {
		fmt.Fprint(os.Stderr, "정말 데이터까지 삭제합니까? 리포트·백업이 사라집니다 [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(os.Stderr, "취소됨")
			return 1
		}
	}
	return app.Uninstall(app.UninstallOptions{Purge: *purge}, app.Deps{})
}
