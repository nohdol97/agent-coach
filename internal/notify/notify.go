// Package notify는 데스크톱 알림을 보낸다 — 사용자가 개선을 인지하는 1차 채널(스펙 R5).
// 전 경로 best-effort: 알림 실패가 분석을 막으면 안 된다.
package notify

import (
	"os"
	"os/exec"
	"runtime"
)

// Send는 OS 네이티브 알림을 시도하고, 실패는 조용히 무시한다.
func Send(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		// 인자 주입 방지 — osascript 스크립트 본문 대신 argv로 전달한다.
		script := `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`
		_ = exec.Command("osascript", "-e", script, title, body).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command("notify-send", "--app-name=agent-coach", title, body).Run()
		}
	case "windows":
		// WinRT 토스트 — 외부 모듈 없이 PowerShell로. 문자열은 env로 넘겨 따옴표 조합 문제를 피한다.
		ps := `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$x = $t.GetElementsByTagName('text')
$x.Item(0).AppendChild($t.CreateTextNode($env:AGENTCOACH_TITLE)) | Out-Null
$x.Item(1).AppendChild($t.CreateTextNode($env:AGENTCOACH_BODY)) | Out-Null
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('agent-coach').Show([Windows.UI.Notifications.ToastNotification]::new($t))`
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
		cmd.Env = append(os.Environ(), "AGENTCOACH_TITLE="+title, "AGENTCOACH_BODY="+body)
		_ = cmd.Run()
	}
}
