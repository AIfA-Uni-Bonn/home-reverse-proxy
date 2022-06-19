package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers {
		fmt.Printf("%s %s\n", container.ID[:10], container.Image)
	}

	mounts := []mount.Mount{{
		Type:   "bind",
		Source: "/Users/ocordes/volatile/public_html",
		Target: "/users/ocordes/public_html",
	},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
		Mounts: mounts,
	}

	config := &container.Config{
		Image:        "registry.gitlab.com/ocordes/userwebsite",
		Env:          []string{fmt.Sprintf("USERNAME=%s", "ocordes")},
		ExposedPorts: nil,
		Hostname:     "test",
	}

	container, err := cli.ContainerCreate(context.Background(), config, hostConfig, nil, nil, "test")

	if err != nil {
		panic(err)
	}

	// Run the actual container
	cli.ContainerStart(context.Background(), container.ID, types.ContainerStartOptions{})
	fmt.Printf("Container %s is created", container.ID)
}
