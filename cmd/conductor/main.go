package main

import (
	"context"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func main() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		log.Fatalln("No connections\n", err)
	}

	containers, err := cli.ContainerList(
		ctx,
		types.ContainerListOptions{All: true},
	)
	if err != nil {
		log.Fatalln("No containers\n", err)
	}
	for _, container := range containers {
		log.Println(container.ID, container.State, container.Image)
	}
}
