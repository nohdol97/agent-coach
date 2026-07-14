// agentcoach — agentsview 기반 일일 에이전트 사용 코칭 CLI.
//
// 서브커맨드: install / analyze / report / uninstall / version (스펙 docs/specs/2026-07-14-mvp.md).
package main

import (
	"fmt"
	"os"
)

var version = "0.1.0" // 릴리스 빌드에서 -ldflags "-X main.version=..."로 주입

const usageText = `agentcoach — AI 에이전트 사용을 매일 분석해 토큰 효율 지침을 자동 기입합니다.

사용법:
  agentcoach                                            (무인자·더블클릭) 미설치면 자동 설치, 설치됐으면 즉시 분석
  agentcoach install [--time HH:MM] [--no-agentsview]   설치: agentsview 확보·OS 스케줄 등록·초기화
  agentcoach analyze [--force] [--dry-run]              일일 분석 실행 (스케줄러가 자동 호출)
  agentcoach report  [--date YYYY-MM-DD]                분석 리포트 출력 (기본: 최신)
  agentcoach uninstall [--purge]                        스케줄 해제·관리 블록 제거
  agentcoach version                                    버전 표시
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return cmdAuto()
	}
	switch args[0] {
	case "install":
		return cmdInstall(args[1:])
	case "analyze":
		return cmdAnalyze(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "uninstall":
		return cmdUninstall(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("agentcoach %s\n", version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 명령: %s\n\n%s", args[0], usageText)
		return 2
	}
}
