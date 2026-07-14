// agentcoach — agentsview 기반 일일 에이전트 사용 코칭 CLI.
// 서브커맨드(install / analyze / report / uninstall)는 MVP 스펙 확정 후 구현한다.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-dev"

func main() {
	fmt.Printf("agentcoach %s — 스캐폴딩 단계, 구현 전입니다.\n", version)
	os.Exit(0)
}
