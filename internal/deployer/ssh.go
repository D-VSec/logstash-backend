package deployer

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Request struct {
	Host          string
	User          string
	Auth          string
	IdentityFile  string
	InstallScript string
	ConfigFile    string
	Teardown      bool
}

type SSHDeployer struct {
	Stdout io.Writer
	Stderr io.Writer
}

func NewSSHDeployer(stdout, stderr io.Writer) *SSHDeployer {
	return &SSHDeployer{Stdout: stdout, Stderr: stderr}
}

func (deployer *SSHDeployer) Deploy(ctx context.Context, request Request) error {
	if request.Auth != "private-key" && request.Auth != "password" {
		return fmt.Errorf("unsupported SSH authentication method %q", request.Auth)
	}
	if request.Auth == "private-key" {
		if _, err := os.Stat(request.IdentityFile); err != nil {
			return fmt.Errorf("identity file: %w", err)
		}
	}

	script, err := os.Open(filepath.Clean(request.InstallScript))
	if err != nil {
		return fmt.Errorf("open install script: %w", err)
	}
	defer script.Close()
	config, err := os.ReadFile(filepath.Clean(request.ConfigFile))
	if err != nil {
		return fmt.Errorf("read Fluent Bit config: %w", err)
	}
	installScript, err := io.ReadAll(script)
	if err != nil {
		return fmt.Errorf("read install script: %w", err)
	}

	target := request.User + "@" + request.Host
	args := []string{}
	if request.Auth == "private-key" {
		args = append(args, "-o", "BatchMode=yes", "-i", request.IdentityFile)
	} else {
		args = append(args, "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no")
	}
	args = append(args, target, "sudo", "bash", "-s")
	command := exec.CommandContext(ctx, "ssh", args...)
	command.Stdin = bytes.NewReader(remoteSetupScript(installScript, config, request.Teardown))
	command.Stdout = deployer.Stdout
	command.Stderr = deployer.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("deploy Fluent Bit to %s: %w", target, err)
	}
	return nil
}

func remoteSetupScript(installScript, config []byte, teardown bool) []byte {
	encodedInstallScript := base64.StdEncoding.EncodeToString(installScript)
	encodedConfig := base64.StdEncoding.EncodeToString(config)

	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	if teardown {
		script.WriteString("systemctl stop fluent-bit 2>/dev/null || true\n")
		script.WriteString("systemctl disable fluent-bit 2>/dev/null || true\n")
		script.WriteString("DEBIAN_FRONTEND=noninteractive apt-get remove -y fluent-bit 2>/dev/null || true\n")
		script.WriteString("rm -rf /etc/fluent-bit\n")
		script.WriteString("systemctl daemon-reload\n")
	}
	script.WriteString("printf '%s' '")
	script.WriteString(encodedInstallScript)
	script.WriteString("' | base64 --decode | bash\n")
	script.WriteString("install -d -m 0750 /var/lib/fluent-bit /var/log/fluent-bit-storage\n")
	script.WriteString("printf '%s' '")
	script.WriteString(encodedConfig)
	script.WriteString("' | base64 --decode > /etc/fluent-bit/fluent-bit.conf\n")
	script.WriteString("systemctl daemon-reload\n")
	script.WriteString("systemctl enable fluent-bit\n")
	script.WriteString("if ! systemctl restart fluent-bit; then\n")
	script.WriteString("    systemctl status fluent-bit --no-pager --full || true\n")
	script.WriteString("    journalctl -u fluent-bit -n 50 --no-pager || true\n")
	script.WriteString("    exit 1\n")
	script.WriteString("fi\n")
	script.WriteString("systemctl is-active --quiet fluent-bit\n")
	return []byte(script.String())
}
