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
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type DClient struct {
	ctx context.Context
	cli *client.Client
}

func NewDClient(context *context.Context) *DClient {

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	return &DClient{
		ctx: *context,
		cli: cli,
	}
}

func (d *DClient) Pull(imgRef string, authStr string) error {

	//reader, err := d.cli.ImagePull(d.ctx, imgRef, types.ImagePullOptions{RegistryAuth: authStr})
	reader, err := d.cli.ImagePull(d.ctx, imgRef, types.ImagePullOptions{RegistryAuth: authStr})
	if err != nil {
		return err
	}

	defer reader.Close()
	io.Copy(os.Stdout, reader)

	return nil
}

//TODO wait till ready
func (d *DClient) Start(img string, env []string, network string) (error, string) {

	resp, err := d.cli.ContainerCreate(d.ctx, &container.Config{
		Image: img,
		Env:   env,
		Tty:   false,
	}, nil, nil, nil, "")
	if err != nil {
		return err, ""
	}

	if network != "" {
		err = d.NetworkConnect(network, resp.ID)
		if err != nil {
			return err, ""
		}
	}

	//, &container.HostConfig{RestartPolicy: container.RestartPolicy{
	//	Name:              "always",
	//	MaximumRetryCount: 0,
	//}

	if err := d.cli.ContainerStart(d.ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err, ""
	}

	//statusCh, errCh := d.cli.ContainerWait(d.ctx, resp.ID, container.WaitConditionNotRunning)
	//select {
	//case err := <-errCh:
	//	if err != nil {
	//		return err, ""
	//	}
	//case <-statusCh:
	//	fmt.Println("!!!status", statusCh)
	//	break
	//}

	fmt.Println("Here")
	return nil, resp.ID
}

func (d *DClient) NetworkConnect(networkName string, containerID string) error {

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

func (d *DClient) Stop(id string) error {

	if err := d.cli.ContainerStop(d.ctx, id, nil); err != nil {
		return err
	}

	return nil
}

func (d *DClient) Events(onEvent func(message events.Message), onError func(err error)) {

	c1, c2 := d.cli.Events(d.ctx, types.EventsOptions{})

	for {
		select {
		case msg := <-c1:
			onEvent(msg)
		case err := <-c2:
			onError(err)
		}
	}

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
