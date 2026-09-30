package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	name := "LOGSTASH_TEST_R2_VALUE"
	t.Setenv(name, "existing")
	path := filepath.Join(t.TempDir(), "r2.env")
	if err := os.WriteFile(path, []byte("# comment\nexport LOGSTASH_TEST_R2_VALUE=from-file\nLOGSTASH_TEST_R2_OTHER='value'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("load env file: %v", err)
	}
	if got := os.Getenv(name); got != "existing" {
		t.Fatalf("existing value = %q, want existing", got)
	}
	if got := os.Getenv("LOGSTASH_TEST_R2_OTHER"); got != "value" {
		t.Fatalf("file value = %q, want value", got)
	}
}
