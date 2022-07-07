package docker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/events"
)

func (c *Controller) updateHealthStatus(msg events.Message) {
	c.anvilMutex.Lock()

	if anv, ok := c.anvils[msg.ID]; ok {
		if strings.Contains(msg.Status, "healthy") {
			anv.status = Healthy
			c.log.Debug("Now container is healthy")
		} else if strings.Contains(msg.Status, "unhealthy") {
			anv.status = Unhealthy
		} else if strings.Contains(msg.Status, "starting") {
			anv.status = Starting
		} else {
			anv.status = Stopped
			c.log.Debug("Now container is stoped")
		}
	}
	c.anvilMutex.Unlock()

	c.CleanUp()
}

func (c *Controller) CleanUp() {

	c.log.Debug("CleanUp")
	c.RestoreStatus()

	for id, anv := range c.anvils {
		if anv.status == Stopped {
			c.anvilMutex.Lock()
			delete(c.anvils, id)
			c.anvilMutex.Unlock()

			c.log.Debug("Container with ID: ", id, " was deleted from map")
		}
	}

}

func (c *Controller) RestoreStatus() {
	containersConfig := c.docker.ReadEnv()

	for id, config := range containersConfig {
		// TODO save const
		if !strings.Contains(config.Image, "anvil") {
			continue
		}

		c.log.Debug("Continue with image: ", config.Image)
		envs := make(map[string]string)

		for _, item := range config.Env {
			parts := strings.Split(item, "=")
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			value := parts[1]

			envs[key] = value
		}

		var (
			address        string
			login          uint64
			password       string
			publicExchange string
			rpcQueue       string
		)

		if v, ok := envs["AMQP_PUBLISH_EXCHANGE"]; ok {
			publicExchange = v
		} else {
			c.log.Error("Couldn't read AMQP_PUBLISH_EXCHANGE env from container: ", id)
			continue
		}

		if v, ok := envs["AMQP_RPC_QUEUE"]; ok {
			rpcQueue = v
		} else {
			c.log.Error("Couldn't read AMQP_RPC_QUEUE env from container: ", id)
			continue
		}

		if v, ok := envs["MT_ADDRESS"]; ok {
			address = v
		} else {
			c.log.Error("Couldn't read MT_ADDRESS env from container: ", id)
			continue
		}

		if v, ok := envs["MT_LOGIN"]; ok {
			login, _ = strconv.ParseUint(v, 10, 64)
		} else {
			c.log.Error("Couldn't read MT_LOGIN env from container: ", id)
			continue
		}

		if v, ok := envs["MT_PASSWORD"]; ok {
			password = v
		} else {
			c.log.Error("Couldn't read MT_PASSWORD env from container: ", id)
			continue
		}

		anvilHash := anvilHash(address, login, password)

		hl, err := c.docker.HealthStatus(id)
		if err != nil {
			c.log.Error("Couldn't get health status of container: ", id)
			continue
		}
		fmt.Println("Restore status: ", hl)
		status := Stopped
		if hl == "healthy" {
			status = Healthy
		} else if hl == "starting" {
			status = Starting
		} else if hl == "unhealthy" {
			status = Stopped
			c.log.Debug("anvil status stopped: ", hl)
			continue
		} else {
			c.log.Debug("Unknown anvil status: ", hl)
			continue
		}

		anvil := Anvil{
			credsHash: anvilHash,
			queues: Queues{
				rpcQueue:        rpcQueue,
				publishExchange: publicExchange,
			},
			status: status,
		}
		c.anvilMutex.Lock()
		c.anvils[id] = &anvil
		c.anvilMutex.Unlock()
		c.log.Debug("Add anvil with hash:", anvilHash)
	}

}
