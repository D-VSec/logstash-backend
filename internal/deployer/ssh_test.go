package deployer

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewSSHDeployer(t *testing.T) {
	deployer := NewSSHDeployer(nil, nil)
	if deployer == nil {
		t.Fatal("expected deployer")
	}
}

func TestRemoteSetupScript(t *testing.T) {
	script := string(remoteSetupScript([]byte("apt-get update"), []byte("[SERVICE]\n"), Request{Teardown: true, Storage: "local", ArchiverBuild: "none"}, nil, nil, nil))
	encodedInstall := base64.StdEncoding.EncodeToString([]byte("apt-get update"))
	encodedConfig := base64.StdEncoding.EncodeToString([]byte("[SERVICE]\n"))

	for _, expected := range []string{
		"systemctl stop fluent-bit 2>/dev/null || true",
		"systemctl disable fluent-bit 2>/dev/null || true",
		"DEBIAN_FRONTEND=noninteractive apt-get remove -y fluent-bit 2>/dev/null || true",
		"rm -rf /etc/fluent-bit",
		"rm -f /etc/logstash-archiver.env /etc/systemd/system/fluent-bit.service.d/r2.conf",
		"printf '%s' '" + encodedInstall + "' | base64 --decode | bash",
		"install -d -m 0750 /var/lib/fluent-bit /var/log/fluent-bit-storage /var/log/fluent-bit-archive",
		"printf '%s' '" + encodedConfig + "' | base64 --decode > /etc/fluent-bit/fluent-bit.conf",
		"systemctl daemon-reload",
		"systemctl enable fluent-bit",
		"if ! systemctl restart fluent-bit; then",
		"systemctl status fluent-bit --no-pager --full",
		"journalctl -u fluent-bit -n 50 --no-pager",
		"systemctl is-active --quiet fluent-bit",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("remote setup script does not contain %q: %s", expected, script)
		}
	}
}

func TestRemoteSetupScriptCanSkipTeardown(t *testing.T) {
	script := string(remoteSetupScript(nil, nil, Request{Storage: "local", ArchiverBuild: "none"}, nil, nil, nil))
	if strings.Contains(script, "apt-get remove -y fluent-bit") {
		t.Fatalf("setup script should not remove Fluent Bit when teardown is disabled: %s", script)
	}
}
