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
			c.log.Debug("Now container is stopped")
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

func (c *Controller) RestoreStatus() error {
	configs, err := c.docker.Containers([]string{c.settings.ImgRef})
	if err != nil {
		return err
	}
	for id, config := range configs {
		envs := make(map[string]string)
		for _, kv := range config.Env {
			parts := strings.Split(kv, "=")
			if len(parts) < 2 {
				continue
			}
			envs[parts[0]] = parts[1]
		}

		address, ok := envs["MT_ADDRESS"]
		if !ok {
			c.log.Error("MT_ADDRESS env not found on container: ", id)
			continue
		}

		loginValue, ok := envs["MT_LOGIN"]
		if !ok {
			c.log.Error("MT_LOGIN env not found on container: ", id)
			continue
		}
		login, err := strconv.ParseUint(loginValue, 10, 64)
		if !ok {
			c.log.Error("MT_LOGIN env value is not numeric on container: ", id)
			continue
		}

		password, ok := envs["MT_PASSWORD"]
		if !ok {
			c.log.Error("MT_PASSWORD env not found on container: ", id)
			continue
		}

		publicExchange, ok := envs["AMQP_PUBLISH_EXCHANGE"]
		if !ok {
			c.log.Error("AMQP_PUBLISH_EXCHANGE env not found on container: ", id)
			continue
		}

		rpcQueue, ok := envs["AMQP_RPC_QUEUE"]
		if !ok {
			c.log.Error("AMQP_RPC_QUEUE env not found on container: ", id)
			continue
		}

		anvilHash, err := anvilHash(address, login, password)
		if err != nil {
			return err
		}

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
	return nil
}
