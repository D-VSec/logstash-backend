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
				Storage:       request.Storage,
				ArchiverBuild: request.ArchiverBuild,
				ProjectDir:    projectDir(),
				R2Endpoint:    os.Getenv("R2_ENDPOINT"),
				R2Bucket:      os.Getenv("R2_BUCKET"),
				R2Prefix:      os.Getenv("R2_PREFIX"),
				R2AccessKeyID: os.Getenv("R2_ACCESS_KEY_ID"),
				R2SecretKey:   os.Getenv("R2_SECRET_ACCESS_KEY"),
			})
		}),
	}
	if err := cli.NewRootCommandWithServices(os.Stdout, os.Stderr, services).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func projectDir() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return workingDirectory
}
