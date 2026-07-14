// Package paths는 agent-coach의 데이터 디렉토리 규약을 한 곳에 모은다.
// 모든 데이터는 이 머신의 사용자 홈 아래에만 둔다 — 세션 파생 데이터는 머신 밖으로 나가지 않는다(스펙 비목표).
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// DataDir는 상태·설정·리포트가 저장되는 루트다.
// AGENTCOACH_DATA_DIR로 재지정 가능(테스트·특수 환경용).
func DataDir() string {
	if d := os.Getenv("AGENTCOACH_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agentcoach"
	}
	return filepath.Join(home, ".agentcoach")
}

func ConfigPath() string  { return filepath.Join(DataDir(), "config.json") }
func StatePath() string   { return filepath.Join(DataDir(), "state.json") }
func ReportsDir() string  { return filepath.Join(DataDir(), "reports") }
func BackupsDir() string  { return filepath.Join(DataDir(), "backups") }

// Expand는 "~/" 프리픽스를 사용자 홈으로 치환한다. 설정 파일에 절대 경로 대신
// 홈 상대 표기를 두어 머신 간 설정 복사가 안전하게 한다.
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
