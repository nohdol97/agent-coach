package schedule

import (
	"strings"
	"testing"
)

func TestParseHHMM(t *testing.T) {
	if h, m, err := ParseHHMM("09:30"); err != nil || h != 9 || m != 30 {
		t.Fatalf("09:30 파싱 실패: %d:%d %v", h, m, err)
	}
	for _, bad := range []string{"9시", "25:00", "12:60", "12", "ab:cd", ""} {
		if _, _, err := ParseHHMM(bad); err == nil {
			t.Fatalf("%q는 에러여야 함", bad)
		}
	}
}

// C6: OS별 스케줄 산출물이 기대와 일치한다.
func TestSchtasksCreateArgs(t *testing.T) {
	args := SchtasksCreateArgs(`C:\Tools\agentcoach.exe`, "09:30")
	if len(args) != 2 {
		t.Fatalf("DAILY+ONLOGON 2건이어야 함: %d", len(args))
	}
	daily := strings.Join(args[0], " ")
	logon := strings.Join(args[1], " ")
	for _, want := range []string{"/SC DAILY", "/ST 09:30", `"C:\Tools\agentcoach.exe" analyze`, "/F"} {
		if !strings.Contains(daily, want) {
			t.Fatalf("DAILY 인자에 %q 누락: %s", want, daily)
		}
	}
	if !strings.Contains(logon, "/SC ONLOGON") || !strings.Contains(logon, "analyze") {
		t.Fatalf("ONLOGON 인자 불일치: %s", logon)
	}
}

func TestSystemdUnits(t *testing.T) {
	svc := SystemdService("/usr/local/bin/agentcoach")
	if !strings.Contains(svc, "ExecStart=/usr/local/bin/agentcoach analyze") || !strings.Contains(svc, "Type=oneshot") {
		t.Fatalf("service 유닛 불일치:\n%s", svc)
	}
	tm := SystemdTimer("09:30")
	for _, want := range []string{"OnCalendar=*-*-* 09:30:00", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(tm, want) {
			t.Fatalf("timer 유닛에 %q 누락:\n%s", want, tm)
		}
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := LaunchdPlist("/usr/local/bin/agentcoach", 9, 30)
	for _, want := range []string{
		"<string>io.agentcoach.daily</string>",
		"<string>/usr/local/bin/agentcoach</string><string>analyze</string>",
		"<key>Hour</key><integer>9</integer>",
		"<key>Minute</key><integer>30</integer>",
		"<key>RunAtLoad</key><true/>",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("plist에 %q 누락:\n%s", want, p)
		}
	}
}
