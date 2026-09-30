package archiver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOnceCreatesArchiveAndRemovesSpoolFiles(t *testing.T) {
	spoolDir := t.TempDir()
	archiveDir := t.TempDir()
	spoolFile := filepath.Join(spoolDir, "vm-logs.jsonl.1")
	if err := os.WriteFile(spoolFile, []byte(`{"message":"hello"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	worker := Worker{SpoolDir: spoolDir, ArchiveDir: archiveDir}
	if err := worker.RunOnce(); err != nil {
		t.Fatalf("run archiver: %v", err)
	}
	if _, err := os.Stat(spoolFile); !os.IsNotExist(err) {
		t.Fatalf("spool file still exists: %v", err)
	}

	archives, err := filepath.Glob(filepath.Join(archiveDir, "logs_archive_*.tar.gz"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives = %v, err = %v", archives, err)
	}
	archive, err := os.Open(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(compressed)
	header, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "vm-logs.jsonl.1" || !strings.Contains(string(contents), "hello") {
		t.Fatalf("unexpected archive entry %q: %s", header.Name, contents)
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker := Worker{SpoolDir: t.TempDir(), ArchiveDir: t.TempDir()}
	if err := worker.Run(ctx); err != context.Canceled {
		t.Fatalf("run error = %v, want context canceled", err)
	}
}
