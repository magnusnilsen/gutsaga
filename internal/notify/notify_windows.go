//go:build windows

package notify

import (
	"os"
	"os/exec"
)

// Show uses a tray balloon. Title and body travel in environment variables
// so a saga title is not interpolated into the PowerShell script.
func Show(title, body string) error {
	script := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Information
$n.BalloonTipTitle = $env:GUTSAGA_NOTIFY_TITLE
$n.BalloonTipText = $env:GUTSAGA_NOTIFY_BODY
$n.Visible = $true
$n.ShowBalloonTip(8000)
Start-Sleep -Seconds 6
$n.Dispose()
`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.Env = append(os.Environ(),
		"GUTSAGA_NOTIFY_TITLE="+clean(title),
		"GUTSAGA_NOTIFY_BODY="+clean(body),
	)
	return start(cmd)
}
