// Package schedule은 일일 실행을 OS 스케줄러에 등록·해제한다(스펙 R6).
// hook 미사용이 설계 결정이다 — 스케줄러는 어느 CLI 사용자에게든 동일하게 동작한다.
//
// 산출물 생성(인자·유닛·plist)은 순수 함수로 분리해 어느 OS에서든 테스트한다(C6).
// 실제 등록은 GOOS 게이트.
package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	winTaskDaily = "AgentCoach Daily"
	winTaskLogon = "AgentCoach Logon"
	systemdBase  = "agentcoach"
	launchdLabel = "io.agentcoach.daily"
)

// ParseHHMM은 "09:30" 형식을 검증해 (시, 분)을 돌려준다.
func ParseHHMM(s string) (int, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("시각 형식은 HH:MM — 입력: %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("시각 범위 밖(00:00~23:59) — 입력: %q", s)
	}
	return h, m, nil
}

// --- Windows: schtasks --------------------------------------------------

// SchtasksCreateArgs는 등록 명령 2건의 인자를 만든다.
// DAILY(정시) + ONLOGON(로그온 시) 2중 발화 — analyze가 하루 1회 멱등이라 무해하고,
// 정시에 꺼져 있던 머신도 로그온 때 따라잡는다.
func SchtasksCreateArgs(exe, hhmm string) [][]string {
	tr := fmt.Sprintf(`"%s" analyze`, exe)
	return [][]string{
		{"/Create", "/F", "/SC", "DAILY", "/TN", winTaskDaily, "/TR", tr, "/ST", hhmm},
		{"/Create", "/F", "/SC", "ONLOGON", "/TN", winTaskLogon, "/TR", tr},
	}
}

func SchtasksDeleteArgs() [][]string {
	return [][]string{
		{"/Delete", "/F", "/TN", winTaskDaily},
		{"/Delete", "/F", "/TN", winTaskLogon},
	}
}

// --- Linux(Ubuntu): systemd user timer -----------------------------------

func SystemdService(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=agent-coach daily analyze

[Service]
Type=oneshot
ExecStart=%s analyze
`, exe)
}

// SystemdTimer의 Persistent=true가 핵심이다 — 정시에 꺼져 있던 머신이
// 다음 부팅에서 놓친 실행을 따라잡는다(노트북 사용자 전제).
func SystemdTimer(hhmm string) string {
	return fmt.Sprintf(`[Unit]
Description=agent-coach daily timer

[Timer]
OnCalendar=*-*-* %s:00
Persistent=true

[Install]
WantedBy=timers.target
`, hhmm)
}

func systemdUserDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

// --- macOS: launchd -------------------------------------------------------

func LaunchdPlist(exe string, hour, minute int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>analyze</string></array>
	<key>StartCalendarInterval</key>
	<dict><key>Hour</key><integer>%d</integer><key>Minute</key><integer>%d</integer></dict>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`, launchdLabel, exe, hour, minute)
}

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

// --- 등록·해제 -------------------------------------------------------------

// Register는 현재 OS의 스케줄러에 일일 실행을 등록하고 사람이 읽을 설명을 돌려준다.
func Register(exe, hhmm string) (string, error) {
	hour, minute, err := ParseHHMM(hhmm)
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		for _, args := range SchtasksCreateArgs(exe, hhmm) {
			if out, err := exec.Command("schtasks", args...).CombinedOutput(); err != nil {
				return "", fmt.Errorf("schtasks 등록 실패: %w (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		return fmt.Sprintf("Windows 작업 스케줄러 등록: %q(매일 %s) + %q(로그온 시)", winTaskDaily, hhmm, winTaskLogon), nil
	case "linux":
		dir := systemdUserDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, systemdBase+".service"), []byte(SystemdService(exe)), 0o644); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, systemdBase+".timer"), []byte(SystemdTimer(hhmm)), 0o644); err != nil {
			return "", err
		}
		if _, err := exec.LookPath("systemctl"); err != nil {
			return "", fmt.Errorf("systemctl 없음 — systemd 미사용 환경이면 cron 등에 `%s analyze`를 매일 %s에 등록하라", exe, hhmm)
		}
		for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", systemdBase + ".timer"}} {
			if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
				return "", fmt.Errorf("systemctl %s 실패: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
		}
		return fmt.Sprintf("systemd user timer 등록: %s.timer (매일 %s, Persistent)", systemdBase, hhmm), nil
	case "darwin":
		p := launchdPlistPath()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, []byte(LaunchdPlist(exe, hour, minute)), 0o644); err != nil {
			return "", err
		}
		_ = exec.Command("launchctl", "unload", p).Run() // 재등록 대비 — 실패 무해
		if out, err := exec.Command("launchctl", "load", p).CombinedOutput(); err != nil {
			return "", fmt.Errorf("launchctl load 실패: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return fmt.Sprintf("launchd 등록: %s (매일 %s + 로드 시 실행)", launchdLabel, hhmm), nil
	default:
		return "", fmt.Errorf("지원하지 않는 OS: %s", runtime.GOOS)
	}
}

// Unregister는 스케줄 등록을 해제한다. 항목별 실패는 경고로 모아 돌려준다 —
// 절반 성공한 해제를 조용히 성공으로 보고하지 않는다.
func Unregister() (warnings []string) {
	switch runtime.GOOS {
	case "windows":
		for _, args := range SchtasksDeleteArgs() {
			if out, err := exec.Command("schtasks", args...).CombinedOutput(); err != nil {
				warnings = append(warnings, fmt.Sprintf("schtasks 삭제 실패(%s): %s", args[len(args)-1], strings.TrimSpace(string(out))))
			}
		}
	case "linux":
		if out, err := exec.Command("systemctl", "--user", "disable", "--now", systemdBase+".timer").CombinedOutput(); err != nil {
			warnings = append(warnings, fmt.Sprintf("timer 해제 실패: %s", strings.TrimSpace(string(out))))
		}
		for _, f := range []string{systemdBase + ".service", systemdBase + ".timer"} {
			if err := os.Remove(filepath.Join(systemdUserDir(), f)); err != nil && !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("%s 삭제 실패: %v", f, err))
			}
		}
	case "darwin":
		p := launchdPlistPath()
		_ = exec.Command("launchctl", "unload", p).Run()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("plist 삭제 실패: %v", err))
		}
	}
	return warnings
}
