package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/nohdol97/agent-coach/internal/app"
	"github.com/nohdol97/agent-coach/internal/paths"
)

// cmdAuto는 무인자 실행(더블클릭)의 자동 모드다(스펙 R10).
// 비개발자에게는 "터미널에 명령 입력" 자체가 설치 장벽이므로,
// 실행 파일을 여는 것만으로 설치→분석이 완결돼야 한다.
func cmdAuto() int {
	var code int
	if isInstalled() {
		fmt.Println("agent-coach 설치 확인 — 즉시 분석을 실행합니다.")
		code = app.Analyze(app.AnalyzeOptions{Force: true}, app.Deps{})
	} else {
		fmt.Println("agent-coach 첫 실행 — 자동 설치를 시작합니다.")
		code = app.Install(app.InstallOptions{}, app.Deps{})
	}
	pauseIfInteractive(os.Stdin)
	return code
}

// isInstalled의 판정 근거는 config.json이다 — install만이 이 파일을 만든다
// (analyze는 state.json·리포트만 쓴다).
func isInstalled() bool {
	_, err := os.Stat(paths.ConfigPath())
	return err == nil
}

// pauseIfInteractive는 대화형 콘솔에서만 Enter를 기다린다 — 더블클릭으로 열린
// 콘솔 창이 결과를 보여주기 전에 닫히는 것을 막는다. 파이프·스케줄러 호출(비대화형)은
// 기다릴 상대가 없으므로 즉시 종료한다(멈춘 백그라운드 프로세스가 되는 것 방지).
func pauseIfInteractive(stdin *os.File) {
	fi, err := stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("\nEnter 키를 누르면 창이 닫힙니다…")
	_, _ = bufio.NewReader(stdin).ReadString('\n')
}
