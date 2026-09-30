package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const version = "dev"

type Options struct {
	ConfigPath string
	Verbose    bool
}

type Services struct {
	Deployer  Deployer
	Retriever Retriever
}

type DeployRequest struct {
	Host          string
	User          string
	Auth          string
	IdentityFile  string
	InstallScript string
	ConfigFile    string
	Teardown      bool
	Storage       string
	ArchiverBuild string
}

type RetrieveRequest struct {
	Source string
	From   time.Time
	To     time.Time
	Output string
}

type Deployer interface {
	Deploy(context.Context, DeployRequest) error
}

type DeployerFunc func(context.Context, DeployRequest) error

func (function DeployerFunc) Deploy(ctx context.Context, request DeployRequest) error {
	return function(ctx, request)
}

type Retriever interface {
	Retrieve(context.Context, RetrieveRequest) error
}

type deployOptions struct {
	Host          string
	User          string
	Auth          string
	IdentityFile  string
	InstallScript string
	ConfigFile    string
	Teardown      bool
	Storage       string
	ArchiverBuild string
	DryRun        bool
}

type retrieveOptions struct {
	Source string
	From   string
	To     string
	Output string
	DryRun bool
}

func NewRootCommand(stdout, stderr io.Writer) *cobra.Command {
	return NewRootCommandWithServices(stdout, stderr, Services{})
}

func NewRootCommandWithServices(stdout, stderr io.Writer, services Services) *cobra.Command {
	options := &Options{}

	root := &cobra.Command{
		Use:           "logstash",
		Short:         "Archive logs from virtual machines",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().StringVar(&options.ConfigPath, "config", "", "path to the configuration file")
	root.PersistentFlags().BoolVar(&options.Verbose, "verbose", false, "enable verbose output")

	root.AddCommand(
		newDeployCommand(stdout, services.Deployer),
		newRetrieveCommand(stdout, services.Retriever),
		newVersionCommand(stdout),
	)
	return root
}

func newDeployCommand(stdout io.Writer, deployer Deployer) *cobra.Command {
	options := &deployOptions{}

	command := &cobra.Command{
		Use:   "deploy",
		Short: "Install and configure Fluent Bit on a VM",
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if err := validateRequired("host", options.Host, "user", options.User); err != nil {
				return err
			}
			if options.Auth != "private-key" && options.Auth != "password" {
				return errors.New("--auth must be private-key or password")
			}
			if options.Storage != "local" && options.Storage != "r2" {
				return errors.New("--storage must be local or r2")
			}
			if options.ArchiverBuild != "none" && options.ArchiverBuild != "linux-amd64" && options.ArchiverBuild != "linux-arm64" {
				return errors.New("--archiver-build must be linux-amd64, linux-arm64, or none")
			}
			if options.Auth == "private-key" {
				return validateRequired("identity-file", options.IdentityFile)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := DeployRequest{
				Host:          options.Host,
				User:          options.User,
				Auth:          options.Auth,
				IdentityFile:  options.IdentityFile,
				InstallScript: options.InstallScript,
				ConfigFile:    options.ConfigFile,
				Teardown:      options.Teardown,
				Storage:       options.Storage,
				ArchiverBuild: options.ArchiverBuild,
			}
			if options.Storage == "r2" && !cmd.Flags().Changed("config-file") {
				request.ConfigFile = "scripts/fluent-bit-r2.conf"
			}
			if options.DryRun {
				credential := request.Auth
				if request.Auth == "private-key" {
					credential += " " + request.IdentityFile
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "deploy plan: %s Fluent Bit on %s@%s using %s, %s storage, %s archiver, %s, and %s\n", teardownLabel(request.Teardown), request.User, request.Host, credential, request.Storage, request.ArchiverBuild, request.InstallScript, request.ConfigFile)
				return err
			}
			if deployer == nil {
				return errors.New("deploy service is not configured")
			}
			return deployer.Deploy(cmd.Context(), request)
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&options.Host, "host", "", "VM hostname or IP address")
	command.Flags().StringVar(&options.User, "user", "", "SSH user")
	command.Flags().StringVar(&options.Auth, "auth", "private-key", "SSH authentication method: private-key or password")
	command.Flags().StringVar(&options.IdentityFile, "identity-file", "", "path to the SSH private key")
	command.Flags().StringVar(&options.InstallScript, "install-script", "scripts/install-fluentbit.sh", "path to the Fluent Bit installation script")
	command.Flags().StringVar(&options.ConfigFile, "config-file", "scripts/fluent-bit.conf", "path to the Fluent Bit configuration")
	command.Flags().BoolVar(&options.Teardown, "teardown", true, "remove the previous Fluent Bit installation before installing")
	command.Flags().StringVar(&options.Storage, "storage", "local", "log storage target: local or r2")
	command.Flags().StringVar(&options.ArchiverBuild, "archiver-build", "linux-amd64", "archiver build target: linux-amd64, linux-arm64, or none")
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "print the deployment plan without connecting")
	return command
}

func teardownLabel(enabled bool) string {
	if enabled {
		return "tear down and install"
	}
	return "install"
}

func newRetrieveCommand(stdout io.Writer, retriever Retriever) *cobra.Command {
	options := &retrieveOptions{}

	command := &cobra.Command{
		Use:   "retrieve",
		Short: "Retrieve archived logs for a time range",
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if err := validateRequired(
				"source", options.Source,
				"from", options.From,
				"to", options.To,
				"output", options.Output,
			); err != nil {
				return err
			}
			return validateTimeRange(options.From, options.To)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			from, _ := time.Parse(time.RFC3339, options.From)
			to, _ := time.Parse(time.RFC3339, options.To)
			request := RetrieveRequest{
				Source: options.Source,
				From:   from,
				To:     to,
				Output: options.Output,
			}
			if options.DryRun {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "retrieve plan: %s from %s to %s into %s\n", request.Source, request.From.Format(time.RFC3339), request.To.Format(time.RFC3339), request.Output)
				return err
			}
			if retriever == nil {
				return errors.New("retrieve service is not configured")
			}
			return retriever.Retrieve(cmd.Context(), request)
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&options.Source, "source", "", "VM or log source identifier")
	command.Flags().StringVar(&options.From, "from", "", "start time in RFC3339 format")
	command.Flags().StringVar(&options.To, "to", "", "end time in RFC3339 format")
	command.Flags().StringVar(&options.Output, "output", "", "output file or - for stdout")
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "print the retrieval plan without reading storage")
	return command
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintln(stdout, version)
		},
	}
	command.SetOut(stdout)
	return command
}

func validateRequired(values ...string) error {
	for index := 0; index < len(values); index += 2 {
		if strings.TrimSpace(values[index+1]) == "" {
			return errors.New("--" + values[index] + " is required")
		}
	}
	return nil
}

func validateTimeRange(fromValue, toValue string) error {
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return fmt.Errorf("--from must be RFC3339: %w", err)
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return fmt.Errorf("--to must be RFC3339: %w", err)
	}
	if !from.Before(to) {
		return errors.New("--from must be before --to")
	}
	return nil
}
