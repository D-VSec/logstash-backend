package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"logstash/internal/archiver"
	"logstash/internal/storage"
)

func main() {
	spoolDir := flag.String("spool-dir", "/var/log/fluent-bit-archive", "directory containing rotated Fluent Bit files")
	archiveDir := flag.String("archive-dir", "/var/log/fluent-bit-archives", "directory for completed tar.gz archives")
	interval := flag.Duration("interval", time.Minute, "archive scan interval")
	once := flag.Bool("once", false, "process available spool files and exit")
	envFile := flag.String("env-file", "/etc/logstash-archiver.env", "R2 environment file")
	flag.Parse()
	if err := storage.LoadEnvFile(*envFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *envFile == "/etc/logstash-archiver.env" {
		if err := storage.LoadEnvFile(".env"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var uploader archiver.Uploader
	if endpoint := os.Getenv("R2_ENDPOINT"); endpoint != "" {
		var err error
		uploader, err = storage.NewR2Uploader(
			endpoint,
			os.Getenv("R2_BUCKET"),
			os.Getenv("R2_PREFIX"),
			os.Getenv("R2_ACCESS_KEY_ID"),
			os.Getenv("R2_SECRET_ACCESS_KEY"),
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	worker := archiver.Worker{SpoolDir: *spoolDir, ArchiveDir: *archiveDir, Interval: *interval, Uploader: uploader}
	if *once {
		if err := worker.RunOnce(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
