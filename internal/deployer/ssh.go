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
	Storage       string
	R2Endpoint    string
	R2Bucket      string
	R2Prefix      string
	R2AccessKeyID string
	R2SecretKey   string
	ArchiverBuild string
	ProjectDir    string
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
	if request.Storage != "local" && request.Storage != "r2" {
		return fmt.Errorf("unsupported storage target %q", request.Storage)
	}
	if request.Storage == "r2" {
		for name, value := range map[string]string{
			"R2_ENDPOINT":          request.R2Endpoint,
			"R2_BUCKET":            request.R2Bucket,
			"R2_ACCESS_KEY_ID":     request.R2AccessKeyID,
			"R2_SECRET_ACCESS_KEY": request.R2SecretKey,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s must be set for r2 storage", name)
			}
		}
	}
	if request.ArchiverBuild != "none" && request.ArchiverBuild != "linux-amd64" && request.ArchiverBuild != "linux-arm64" {
		return fmt.Errorf("unsupported archiver build target %q", request.ArchiverBuild)
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
	archiverBinary, archiverService, logrotateConfig, err := buildArchiver(ctx, request)
	if err != nil {
		return err
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
	command.Stdin = bytes.NewReader(remoteSetupScript(installScript, config, request, archiverBinary, archiverService, logrotateConfig))
	command.Stdout = deployer.Stdout
	command.Stderr = deployer.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("deploy Fluent Bit to %s: %w", target, err)
	}
	return nil
}

func remoteSetupScript(installScript, config []byte, request Request, archiverBinary, archiverService, logrotateConfig []byte) []byte {
	encodedInstallScript := base64.StdEncoding.EncodeToString(installScript)
	encodedConfig := base64.StdEncoding.EncodeToString(config)

	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	if request.Teardown {
		script.WriteString("systemctl stop fluent-bit 2>/dev/null || true\n")
		script.WriteString("systemctl disable fluent-bit 2>/dev/null || true\n")
		script.WriteString("DEBIAN_FRONTEND=noninteractive apt-get remove -y fluent-bit 2>/dev/null || true\n")
		script.WriteString("rm -rf /etc/fluent-bit\n")
		script.WriteString("systemctl daemon-reload\n")
		script.WriteString("systemctl stop logstash-archiver 2>/dev/null || true\n")
		script.WriteString("systemctl disable logstash-archiver 2>/dev/null || true\n")
	}
	script.WriteString("printf '%s' '")
	script.WriteString(encodedInstallScript)
	script.WriteString("' | base64 --decode | bash\n")
	script.WriteString("install -d -m 0750 /var/lib/fluent-bit /var/log/fluent-bit-storage /var/log/fluent-bit-archive\n")
	script.WriteString("printf '%s' '")
	script.WriteString(encodedConfig)
	script.WriteString("' | base64 --decode > /etc/fluent-bit/fluent-bit.conf\n")
	if len(archiverBinary) > 0 {
		script.WriteString("install -d -m 0750 /var/log/fluent-bit-archive /var/log/fluent-bit-archives\n")
		script.WriteString("printf '%s' '")
		script.WriteString(base64.StdEncoding.EncodeToString(archiverBinary))
		script.WriteString("' | base64 --decode > /usr/local/bin/logstash-archiver\n")
		script.WriteString("chmod 0755 /usr/local/bin/logstash-archiver\n")
		script.WriteString("printf '%s' '")
		script.WriteString(base64.StdEncoding.EncodeToString(archiverService))
		script.WriteString("' | base64 --decode > /etc/systemd/system/logstash-archiver.service\n")
		script.WriteString("printf '%s' '")
		script.WriteString(base64.StdEncoding.EncodeToString(logrotateConfig))
		script.WriteString("' | base64 --decode > /etc/logrotate.d/logstash-fluent-bit\n")
		script.WriteString("systemctl daemon-reload\n")
		script.WriteString("systemctl enable --now logstash-archiver\n")
	}
	if request.Storage == "r2" {
		envFile := "R2_ENDPOINT=" + request.R2Endpoint + "\n" +
			"R2_BUCKET=" + request.R2Bucket + "\n" +
			"R2_PREFIX=" + request.R2Prefix + "\n" +
			"R2_ACCESS_KEY_ID=" + request.R2AccessKeyID + "\n" +
			"R2_SECRET_ACCESS_KEY=" + request.R2SecretKey + "\n"
		encodedEnv := base64.StdEncoding.EncodeToString([]byte(envFile))
		encodedDropIn := base64.StdEncoding.EncodeToString([]byte("[Service]\nEnvironmentFile=-/etc/fluent-bit/r2.env\n"))
		script.WriteString("install -d -m 0750 /etc/fluent-bit /etc/systemd/system/fluent-bit.service.d\n")
		script.WriteString("printf '%s' '")
		script.WriteString(encodedEnv)
		script.WriteString("' | base64 --decode > /etc/fluent-bit/r2.env\n")
		script.WriteString("chmod 0600 /etc/fluent-bit/r2.env\n")
		script.WriteString("printf '%s' '")
		script.WriteString(encodedDropIn)
		script.WriteString("' | base64 --decode > /etc/systemd/system/fluent-bit.service.d/r2.conf\n")
	}
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

func buildArchiver(ctx context.Context, request Request) ([]byte, []byte, []byte, error) {
	if request.ArchiverBuild == "none" {
		return nil, nil, nil, nil
	}
	goarch := strings.TrimPrefix(request.ArchiverBuild, "linux-")
	temporaryDirectory, err := os.MkdirTemp("", "logstash-archiver-build-")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create archiver build directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)
	binaryPath := filepath.Join(temporaryDirectory, "logstash-archiver")
	command := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./cmd/logstash-archiver")
	command.Dir = request.ProjectDir
	command.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build archiver for %s: %w: %s", request.ArchiverBuild, err, strings.TrimSpace(string(output)))
	}
	archiverBinary, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read archiver binary: %w", err)
	}
	archiverService, err := os.ReadFile(filepath.Join(request.ProjectDir, "scripts", "logstash-archiver.service"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read archiver service: %w", err)
	}
	logrotateConfig, err := os.ReadFile(filepath.Join(request.ProjectDir, "scripts", "logrotate-fluent-bit.conf"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read logrotate config: %w", err)
	}
	return archiverBinary, archiverService, logrotateConfig, nil
}
