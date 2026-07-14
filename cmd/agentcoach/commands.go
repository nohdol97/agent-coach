package main

import (
	"fmt"
	"os"
)

// 각 명령의 실제 구현은 internal 패키지 완성에 맞춰 결선한다(단계별 커밋).
func notImplemented(name string) int {
	fmt.Fprintf(os.Stderr, "%s: 아직 구현되지 않았습니다 (스펙 docs/specs/2026-07-14-mvp.md 진행 중)\n", name)
	return 1
}

func cmdInstall(args []string) int   { return notImplemented("install") }
func cmdAnalyze(args []string) int   { return notImplemented("analyze") }
func cmdReport(args []string) int    { return notImplemented("report") }
func cmdUninstall(args []string) int { return notImplemented("uninstall") }
