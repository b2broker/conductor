package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type Client struct {
	ctx context.Context
	cli *client.Client
}

func NewClient(ctx context.Context) *Client {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	return &Client{
		ctx: ctx,
		cli: cli,
	}
}

func (d *Client) Pull(imgRef string, authStr string) error {
	reader, err := d.cli.ImagePull(d.ctx, imgRef, types.ImagePullOptions{RegistryAuth: authStr})
	if err != nil {
		return err
	}

	defer reader.Close()
	io.Copy(os.Stdout, reader)

	return nil
}

func (d *Client) Create(img string, envs []string) (id string, err error) {
	info, err := d.cli.ContainerCreate(d.ctx,
		&container.Config{
			Image: img,
			Env:   envs,
			Tty:   false,
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{
				Name:              "on-failure",
				MaximumRetryCount: 100,
			},
		},
		// TODO: shouldn't we describe network here?
		nil,
		nil,
		// TODO: It's appropriate to name container anvil-{version}-{login}-{host}
		"",
	)
	if err != nil {
		return
	}
	id = info.ID
	return
}

func (d *Client) Start(id string, network string) (string, error) {
	if network != "" {
		err := d.NetworkConnect(network, id)
		if err != nil {
			return "", err
		}
	}

	if err := d.cli.ContainerStart(d.ctx, id, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	return id, nil
}

func (d *Client) NetworkConnect(networkName string, containerID string) error {
	instance, err := d.cli.ContainerInspect(d.ctx, containerID)
	if err != nil {
		return err
	}
	if _, ok := instance.NetworkSettings.Networks[networkName]; ok {
		return nil
	}

	settings := network.EndpointSettings{
		Links:     []string{"rabbitmq"},
		NetworkID: networkName,
	}

	err = d.cli.NetworkConnect(d.ctx, networkName, containerID, &settings)
	if err != nil {
		return err
	}
	return nil
}

func (d *Client) Stop(id string) error {
	return d.cli.ContainerStop(d.ctx, id, nil)
}

func (d *Client) Events(onEvent func(message events.Message), onError func(err error), images []string) {
	opts := types.EventsOptions{}
	opts.Filters = filters.NewArgs()
	opts.Filters.Add("type", events.ContainerEventType)
	for _, image := range images {
		opts.Filters.Add("image", image)
	}

	events, errors := d.cli.Events(d.ctx, opts)

	for {
		select {
		case msg := <-events:
			onEvent(msg)
		case err := <-errors:
			onError(err)
		}
	}
}

func (d *Client) HealthStatus(id string) (string, error) {
	res, err := d.cli.ContainerInspect(d.ctx, id)
	if err != nil {
		return "", err
	}
	if res.State.Health == nil {
		return string(Stopped), nil
	}

	return res.State.Health.Status, nil
}

func (d *Client) Containers(images []string) (map[string]container.Config, error) {
	opts := types.ContainerListOptions{All: true}
	opts.Filters = filters.NewArgs()
	for _, image := range images {
		opts.Filters.Add("ancestor", image)
	}

	containers, err := d.cli.ContainerList(d.ctx, opts)
	if err != nil {
		return nil, err
	}

	containersList := make(map[string]container.Config)

	for _, container := range containers {
		res, err := d.cli.ContainerInspect(d.ctx, container.ID)
		if err != nil {
			fmt.Println("info can't be read on container: ", container.ID)
			continue
		}
		containersList[container.ID] = *res.Config
	}

	return containersList, nil
}

func AuthSrt(user string, pass string) string {
	authConfig := types.AuthConfig{
		Username: user,
		Password: pass,
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		panic(err)
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	return authStr
}
