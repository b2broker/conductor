package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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

func NewClient(ctx context.Context) (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		ctx: ctx,
		cli: cli,
	}, nil
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

func (d *Client) Start(img string, env []string, network string) (string, error) {
	resp, err := d.cli.ContainerCreate(d.ctx, &container.Config{
		Image: img,
		Env:   env,
		Tty:   false,
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 3,
		},
	}, nil, nil, "")
	if err != nil {
		return "", err
	}

	if network != "" {
		err = d.NetworkConnect(network, resp.ID)
		if err != nil {
			return "", err
		}
	}

	if err := d.cli.ContainerStart(d.ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (d *Client) NetworkConnect(networkName string, containerID string) error {
	settings := network.EndpointSettings{
		Links:     []string{"rabbitmq"},
		NetworkID: networkName,
	}

	err := d.cli.NetworkConnect(d.ctx, networkName, containerID, &settings)
	if err != nil {
		return err
	}
	return nil
}

func (d *Client) Stop(id string) error {
	if err := d.cli.ContainerStop(d.ctx, id, nil); err != nil {
		return err
	}

	if err := d.cli.ContainerRemove(d.ctx, id, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}); err != nil {
		return err
	}

	return nil
}

// func (d *Client) Events(onEvent func(message events.Message), onError func(err error)) {
// 	c1, c2 := d.cli.Events(d.ctx, types.EventsOptions{})

// 	for {
// 		select {
// 		case msg := <-c1:
// 			onEvent(msg)
// 		case err := <-c2:
// 			onError(err)
// 		}
// 	}
// }

func (d *Client) HealthStatus(id string) (string, error) {

	res, err := d.cli.ContainerInspect(d.ctx, id)
	if err != nil {
		return "", err
	}

	return res.State.Health.Status, nil
}

func (d *Client) ReadEnv() map[string]container.Config {
	containers, err := d.cli.ContainerList(d.ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	containersList := make(map[string]container.Config)

	for _, c := range containers {

		res, err := d.cli.ContainerInspect(d.ctx, c.ID)
		if err != nil {
			fmt.Println("Can't read info about ", c.ID)
			continue
		}

		containersList[c.ID] = *res.Config
	}

	return containersList
}

func (d *Client) Containers(images []string) (Resources, error) {
	opts := types.ContainerListOptions{All: true}
	opts.Filters = filters.NewArgs()
	for _, image := range images {
		opts.Filters.Add("ancestor", image)
	}
	containers, err := d.cli.ContainerList(d.ctx, opts)
	if err != nil {
		return nil, err
	}
	resources := make(Resources, len(containers))
	for _, container := range containers {
		info, err := d.cli.ContainerInspect(d.ctx, container.ID)
		if err != nil {
			log.Println(err)
			continue
		}
		instance := NewInstance(info)
		address, ok := instance.Env("MT_ADDRESS")
		if !ok {
			continue
		}
		login, ok := instance.Env("MT_LOGIN")
		if !ok {
			continue
		}
		password, ok := instance.Env("MT_PASSWORD")
		if !ok {
			continue
		}

		if _, ok := instance.Env("AMQP_PUBLISH_EXCHANGE"); !ok {
			continue
		}
		if _, ok := instance.Env("AMQP_RPC_QUEUE"); !ok {
			continue
		}

		hash, err := Hash(
			strings.Join(address, ""),
			strings.Join(login, ""),
			strings.Join(password, ""),
		)
		if err != nil {
			log.Println(err)
			continue
		}
		resources.Add(instance, hash)
	}
	return resources, nil
}

func (d *Client) Events(images []string) (<-chan events.Message, <-chan error) {
	opts := types.EventsOptions{}
	opts.Filters = filters.NewArgs()
	opts.Filters.Add("type", events.ContainerEventType)
	for _, image := range images {
		opts.Filters.Add("image", image)
	}
	return d.cli.Events(d.ctx, opts)
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
