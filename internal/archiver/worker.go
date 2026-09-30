package archiver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Worker struct {
	SpoolDir   string
	ArchiveDir string
	Interval   time.Duration
	Uploader   Uploader
}

type Uploader interface {
	Upload(context.Context, string, io.Reader) error
}

func (worker Worker) Run(ctx context.Context) error {
	if worker.Interval <= 0 {
		worker.Interval = time.Minute
	}

	if err := worker.runOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(worker.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := worker.runOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (worker Worker) RunOnce() error {
	return worker.runOnce(context.Background())
}

func (worker Worker) runOnce(ctx context.Context) error {
	if err := os.MkdirAll(worker.SpoolDir, 0750); err != nil {
		return fmt.Errorf("create spool directory: %w", err)
	}
	entries, err := os.ReadDir(worker.SpoolDir)
	if err != nil {
		return fmt.Errorf("read spool directory: %w", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.Contains(entry.Name(), ".jsonl.") {
			continue
		}
		files = append(files, filepath.Join(worker.SpoolDir, entry.Name()))
	}
	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)
	if err := os.MkdirAll(worker.ArchiveDir, 0750); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	now := time.Now().UTC()
	name := fmt.Sprintf("logs_archive_%s.tar.gz", now.Format("20060102_150405"))
	temporaryPath := filepath.Join(worker.ArchiveDir, "."+name+".tmp")
	archivePath := filepath.Join(worker.ArchiveDir, name)
	if err := createArchive(temporaryPath, files); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, archivePath); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("publish archive: %w", err)
	}
	if worker.Uploader != nil {
		archive, err := os.Open(archivePath)
		if err != nil {
			return fmt.Errorf("open archive for upload: %w", err)
		}
		uploadErr := worker.Uploader.Upload(ctx, filepath.Base(archivePath), archive)
		closeErr := archive.Close()
		if uploadErr != nil {
			return fmt.Errorf("upload archive: %w", uploadErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close archive after upload: %w", closeErr)
		}
		if err := os.Remove(archivePath); err != nil {
			return fmt.Errorf("remove uploaded archive: %w", err)
		}
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("remove archived spool file %s: %w", file, err)
		}
	}
	return nil
}

func createArchive(destination string, files []string) error {
	archive, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer archive.Close()
	compressed := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(compressed)
	for _, file := range files {
		if err := addFile(tarWriter, file); err != nil {
			_ = tarWriter.Close()
			_ = compressed.Close()
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar archive: %w", err)
	}
	if err := compressed.Close(); err != nil {
		return fmt.Errorf("close gzip archive: %w", err)
	}
	return nil
}

func addFile(writer *tar.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open spool file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat spool file: %w", err)
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}
	header.Name = filepath.Base(path)
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("write spool file: %w", err)
	}
	return nil
}
