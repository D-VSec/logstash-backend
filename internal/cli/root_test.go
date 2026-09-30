package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type deployerFunc func(context.Context, DeployRequest) error

func (function deployerFunc) Deploy(ctx context.Context, request DeployRequest) error {
	return function(ctx, request)
}

type retrieverFunc func(context.Context, RetrieveRequest) error

func (function retrieverFunc) Retrieve(ctx context.Context, request RetrieveRequest) error {
	return function(ctx, request)
}

func TestVersionCommand(t *testing.T) {
	var output bytes.Buffer
	command := NewRootCommand(&output, &output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if got := strings.TrimSpace(output.String()); got != version {
		t.Fatalf("version output = %q, want %q", got, version)
	}
}

func TestDeployRequiresSSHOptions(t *testing.T) {
	command := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"deploy"})

	err := command.Execute()
	if err == nil || err.Error() != "--host is required" {
		t.Fatalf("execute deploy error = %v, want missing host error", err)
	}
}

func TestDeployDryRun(t *testing.T) {
	var output bytes.Buffer
	command := NewRootCommand(&output, &output)
	command.SetArgs([]string{
		"deploy",
		"--host", "vm.example.com",
		"--user", "ubuntu",
		"--identity-file", "/tmp/id_ed25519",
		"--dry-run",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute deploy dry-run: %v", err)
	}
	if !strings.Contains(output.String(), "deploy plan: tear down and install Fluent Bit on ubuntu@vm.example.com") {
		t.Fatalf("unexpected dry-run output: %q", output.String())
	}
}

func TestDeployPasswordAuthDoesNotRequireIdentityFile(t *testing.T) {
	var output bytes.Buffer
	command := NewRootCommand(&output, &output)
	command.SetArgs([]string{
		"deploy",
		"--host", "vm.example.com",
		"--user", "ubuntu",
		"--auth", "password",
		"--dry-run",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute password deploy dry-run: %v", err)
	}
	if !strings.Contains(output.String(), "using password") {
		t.Fatalf("unexpected password dry-run output: %q", output.String())
	}
}

func TestDeployRejectsUnknownAuth(t *testing.T) {
	command := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{
		"deploy",
		"--host", "vm.example.com",
		"--user", "ubuntu",
		"--auth", "token",
	})

	err := command.Execute()
	if err == nil || err.Error() != "--auth must be private-key or password" {
		t.Fatalf("execute deploy error = %v, want invalid auth error", err)
	}
}

func TestRetrieveDelegatesParsedRequest(t *testing.T) {
	var received RetrieveRequest
	command := NewRootCommandWithServices(&bytes.Buffer{}, &bytes.Buffer{}, Services{
		Retriever: retrieverFunc(func(_ context.Context, request RetrieveRequest) error {
			received = request
			return nil
		}),
	})
	command.SetArgs([]string{
		"retrieve",
		"--source", "vm-1",
		"--from", "2026-09-30T10:00:00Z",
		"--to", "2026-09-30T11:00:00Z",
		"--output", "-",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute retrieve: %v", err)
	}
	if received.Source != "vm-1" || received.Output != "-" {
		t.Fatalf("unexpected request: %+v", received)
	}
	if !received.From.Equal(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected from time: %s", received.From)
	}
}

func TestRetrieveRejectsReversedTimeRange(t *testing.T) {
	command := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{
		"retrieve",
		"--source", "vm-1",
		"--from", "2026-09-30T11:00:00Z",
		"--to", "2026-09-30T10:00:00Z",
		"--output", "-",
	})

	err := command.Execute()
	if err == nil || err.Error() != "--from must be before --to" {
		t.Fatalf("execute retrieve error = %v, want reversed range error", err)
	}
}
