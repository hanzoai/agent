package aws

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"text/template"
)

// UserDataParams holds the template parameters for instance bootstrap scripts.
type UserDataParams struct {
	ControlPlaneURL string
	APIKey          string
	InstanceID      string
	BotPackage      string
	BotVersion      string
}

var linuxBootstrapTmpl = template.Must(template.New("linux").Parse(`#!/bin/bash
set -euo pipefail

export HANZO_AGENTS_SERVER_URL="{{.ControlPlaneURL}}"
export HANZO_AGENTS_API_KEY="{{.APIKey}}"
export HANZO_AGENTS_INSTANCE_ID="{{.InstanceID}}"

# Install Python and agent SDK
if ! command -v python3 &>/dev/null; then
  apt-get update -qq && apt-get install -y -qq python3 python3-pip curl
fi

pip3 install --quiet hanzo-agents
{{- if .BotPackage}}
hanzo-agents install "{{.BotPackage}}"{{if .BotVersion}} --version "{{.BotVersion}}"{{end}}
hanzo-agents run "{{.BotPackage}}" &
{{- end}}
`))

var macOSBootstrapTmpl = template.Must(template.New("macos").Parse(`#!/bin/bash
set -euo pipefail

export HANZO_AGENTS_SERVER_URL="{{.ControlPlaneURL}}"
export HANZO_AGENTS_API_KEY="{{.APIKey}}"
export HANZO_AGENTS_INSTANCE_ID="{{.InstanceID}}"

# Enable Screen Sharing for VNC access
sudo /System/Library/CoreServices/RemoteManagement/ARDAgent.app/Contents/Resources/kickstart \
  -activate -configure -access -on -restart -agent -privs -all

# Install agent SDK
pip3 install --quiet hanzo-agents
{{- if .BotPackage}}
hanzo-agents install "{{.BotPackage}}"{{if .BotVersion}} --version "{{.BotVersion}}"{{end}}
hanzo-agents run "{{.BotPackage}}" &
{{- end}}
`))

var windowsBootstrapTmpl = template.Must(template.New("windows").Parse(`<powershell>
$ErrorActionPreference = "Stop"

$env:HANZO_AGENTS_SERVER_URL = "{{.ControlPlaneURL}}"
$env:HANZO_AGENTS_API_KEY = "{{.APIKey}}"
$env:HANZO_AGENTS_INSTANCE_ID = "{{.InstanceID}}"

# Set persistent env vars
[Environment]::SetEnvironmentVariable("HANZO_AGENTS_SERVER_URL", "{{.ControlPlaneURL}}", "Machine")
[Environment]::SetEnvironmentVariable("HANZO_AGENTS_API_KEY", "{{.APIKey}}", "Machine")
[Environment]::SetEnvironmentVariable("HANZO_AGENTS_INSTANCE_ID", "{{.InstanceID}}", "Machine")

# Install Python if not present
if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    Invoke-WebRequest -Uri "https://www.python.org/ftp/python/3.12.0/python-3.12.0-amd64.exe" -OutFile "$env:TEMP\python-installer.exe"
    Start-Process -Wait -FilePath "$env:TEMP\python-installer.exe" -ArgumentList "/quiet", "InstallAllUsers=1", "PrependPath=1"
    $env:PATH = [Environment]::GetEnvironmentVariable("PATH", "Machine")
}

# Install agent SDK
pip install --quiet hanzo-agents
{{- if .BotPackage}}
hanzo-agents install "{{.BotPackage}}"{{if .BotVersion}} --version "{{.BotVersion}}"{{end}}
Start-Process -NoNewWindow -FilePath "hanzo-agents" -ArgumentList "run", "{{.BotPackage}}"
{{- end}}
</powershell>
`))

// RenderUserData renders the bootstrap script and returns it base64-encoded.
func RenderUserData(platform string, params UserDataParams) (string, error) {
	var tmpl *template.Template
	switch platform {
	case "linux":
		tmpl = linuxBootstrapTmpl
	case "macos":
		tmpl = macOSBootstrapTmpl
	case "windows":
		tmpl = windowsBootstrapTmpl
	default:
		return "", fmt.Errorf("unsupported platform for userdata: %s", platform)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to render userdata template: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
