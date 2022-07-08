package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
)

type ContainerState string

const (
	Created    ContainerState = "created"
	Dead       ContainerState = "dead"
	Exited     ContainerState = "exited"
	Paused     ContainerState = "paused"
	Removing   ContainerState = "removing"
	Restarting ContainerState = "restarting"
	Running    ContainerState = "running"
)

type ContainerHealthStatus string

const (
	NoHealthcheck ContainerHealthStatus = types.NoHealthcheck
	Starting      ContainerHealthStatus = types.Starting
	Healthy       ContainerHealthStatus = types.Healthy
	Unhealthy     ContainerHealthStatus = types.Unhealthy
)

type Instance struct {
	types.ContainerJSON

	envsMu sync.RWMutex
	envs   url.Values
}

func NewInstance(container types.ContainerJSON) *Instance {
	return &Instance{
		ContainerJSON: container,
		envs:          make(url.Values),
	}
}

func (i *Instance) ID() string {
	return i.ContainerJSON.ID
}

func (i *Instance) State() ContainerState {
	return ContainerState(i.ContainerJSON.State.Status)
}

func (i *Instance) Status() ContainerHealthStatus {
	return ContainerHealthStatus(i.ContainerJSON.State.Health.Status)
}

func (i *Instance) Envs() (url.Values, error) {
	i.envsMu.Lock()
	defer i.envsMu.Unlock()

	if len(i.envs) == 0 {
		envs, err := url.ParseQuery(strings.Join(i.ContainerJSON.Config.Env, "&"))
		if err != nil {
			return make(map[string][]string), err
		}
		i.envs = envs
	}

	return i.envs, nil
}

func (i *Instance) Env(name string) (values []string, ok bool) {
	i.envsMu.Lock()
	defer i.envsMu.Unlock()

	if len(i.envs) == 0 && len(i.ContainerJSON.Config.Env) > 0 {
		if _, err := i.Envs(); err == nil {
			return nil, false
		}
	}

	values, ok = i.envs[name]
	return
}

type InstanceHash string

type Resources map[InstanceHash][]*Instance

func (rs Resources) Add(instance *Instance, hash InstanceHash) {
	rs[hash] = append(rs[hash], instance)
}

func Hash(parts ...string) (InstanceHash, error) {
	hasher := sha256.New()
	if _, err := hasher.Write([]byte(strings.Join(parts, ""))); err != nil {
		return "", err
	}
	return InstanceHash(hex.EncodeToString(hasher.Sum(nil))), nil
}
