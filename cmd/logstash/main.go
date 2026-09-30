package main

import (
	"context"
	"fmt"
	"os"

	"logstash/internal/cli"
	"logstash/internal/deployer"
)

func main() {
	sshDeployer := deployer.NewSSHDeployer(os.Stdout, os.Stderr)
	services := cli.Services{
		Deployer: cli.DeployerFunc(func(ctx context.Context, request cli.DeployRequest) error {
			return sshDeployer.Deploy(ctx, deployer.Request{
				Host:          request.Host,
				User:          request.User,
				Auth:          request.Auth,
				IdentityFile:  request.IdentityFile,
				InstallScript: request.InstallScript,
				ConfigFile:    request.ConfigFile,
				Teardown:      request.Teardown,
			})
		}),
	}
	if err := cli.NewRootCommandWithServices(os.Stdout, os.Stderr, services).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
