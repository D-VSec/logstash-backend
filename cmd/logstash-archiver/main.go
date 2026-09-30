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
)

func main() {
	spoolDir := flag.String("spool-dir", "/var/log/fluent-bit-archive", "directory containing rotated Fluent Bit files")
	archiveDir := flag.String("archive-dir", "/var/log/fluent-bit-archives", "directory for completed tar.gz archives")
	interval := flag.Duration("interval", time.Minute, "archive scan interval")
	once := flag.Bool("once", false, "process available spool files and exit")
	flag.Parse()

	worker := archiver.Worker{SpoolDir: *spoolDir, ArchiveDir: *archiveDir, Interval: *interval}
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
