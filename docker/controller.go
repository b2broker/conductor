package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/b2broker/conductor/rabbitmq"
	"github.com/docker/docker/api/types/events"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

type Settings struct {
	AmqpHost         string
	AuthToken        string
	ImgRef           string
	NetworkName      string
	AmqpExternalName string
	StartTimeout     int
}

type Controller struct {
	docker   *Client
	anvils   map[string]*Anvil
	rabbit   *rabbitmq.Rabbit
	settings Settings
	log      *zap.SugaredLogger

	eventState   map[string]chan events.Message
	eventStateMu sync.RWMutex
	anvilMutex   sync.RWMutex
}

func NewController(d *Client, r *rabbitmq.Rabbit, settings Settings, logger *zap.SugaredLogger) *Controller {
	return &Controller{
		docker:     d,
		rabbit:     r,
		settings:   settings,
		anvils:     make(map[string]*Anvil),
		log:        logger,
		eventState: make(map[string]chan events.Message),
	}
}

func (c *Controller) findAnvil(hash string) (string, *Anvil, error) {

	c.anvilMutex.RLock()
	defer c.anvilMutex.RUnlock()

	for dockerId, anvil := range c.anvils {
		if anvil.credsHash == hash {
			return dockerId, anvil, nil
		}
	}

	return "", &Anvil{}, fmt.Errorf("couldn't find Anvil")

}

func (c *Controller) findOrCreate(request AnvilRequest) (Queues, error) {

	hash := anvilHash(request.server, request.login, request.password)
	c.log.Debugf("start new anvil. server: %s login: %d password: %s", request.server, request.login, request.password)

	err := c.StartAndWait(request, hash)
	if err != nil {
		c.log.Error("error while start Anvil", err)
		return Queues{}, err
	}

	_, anvil, err := c.findAnvil(hash)

	if err != nil {
		c.log.Error("error after start Anvil", err)
		return Queues{}, err
	}

	if anvil.status != Healthy {
		c.log.Error("Anvil unhealthy")
		return Queues{}, fmt.Errorf("anvil unhealthy")
	}

	return anvil.queues, nil
}

func (c *Controller) processStop(request AnvilRequest, corId string, replyTo string) {
	hash := anvilHash(request.server, request.login, request.password)
	id, anvil, err := c.findAnvil(hash)

	if err != nil {
		c.log.Debug("Couldn't stop Anvil. Anvil doesn't exist")
		c.sendStopResponse(err.Error(), corId, replyTo)
		return
	}

	c.log.Debug("Need to Stop Anvil ", anvil.credsHash)

	errSrt := ""

	err = c.docker.Stop(id)
	if err != nil {
		errSrt = err.Error()
	} else {
		c.anvilMutex.Lock()
		delete(c.anvils, id)
		c.anvilMutex.Unlock()
	}

	c.sendStopResponse(errSrt, corId, replyTo)

}

func (c *Controller) processCreate(amqpMsg amqp.Delivery) {

	request, err := parseRequest(amqpMsg)
	if err != nil {
		c.log.Error("Couldn't parse request")
		return
	}

	queues, err := c.findOrCreate(request)

	c.sendStartResponse(queues, amqpMsg.CorrelationId, amqpMsg.ReplyTo, err)
}

func (c *Controller) Handler(amqpMsg amqp.Delivery) {

	c.log.Debugw("Get request",
		"CorId", amqpMsg.CorrelationId,
		"ReplyTo", amqpMsg.ReplyTo)

	if v, k := amqpMsg.Headers["endpoint"]; k {
		c.log.Debug("Endpoint", v)
		if v == "ConductorService.Attach" {
			go c.processCreate(amqpMsg)

		} else if v == "ConductorService.Detach" {
			request, err := parseRequest(amqpMsg)
			if err == nil {
				c.processStop(request, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
			}
		}
	}
}

func (c *Controller) PullAnvil() error {
	err := c.docker.Pull(c.settings.ImgRef, c.settings.AuthToken)
	if err != nil {
		c.log.Errorw("Can't pull image", "error", err.Error())
		return err
	}
	return nil
}

func (c *Controller) startAnvil(request AnvilRequest) (string, Queues, error) {

	c.log.Debug("startAnvil")
	rpcQueue := uuid.New().String()
	publishExchange := uuid.New().String()

	env := []string{fmt.Sprintf("MT_ADDRESS=%s", request.server),
		fmt.Sprintf("MT_LOGIN=%d", request.login),
		fmt.Sprintf("MT_PASSWORD=%s", request.password),
		fmt.Sprintf("AMQP_HOST=%s", c.settings.AmqpExternalName),
		fmt.Sprintf("AMQP_RPC_QUEUE=%s", rpcQueue),
		fmt.Sprintf("AMQP_PUBLISH_EXCHANGE=%s", publishExchange)}

	resID, err := c.docker.Start(c.settings.ImgRef, env, c.settings.NetworkName)
	if err != nil {
		c.log.Errorw("Can't start container", "error", err.Error())
		return "", Queues{}, err
	}

	queues := Queues{
		rpcQueue:        rpcQueue,
		publishExchange: publishExchange,
	}

	c.log.Debug("Return from startAnvil")
	return resID, queues, nil
}

func (c *Controller) reply(answer []byte, corId string, replyTo string) error {

	fmt.Println("Sent answer. CorId: ", corId, " ReplyTo: ", replyTo)

	amqpMsg := amqp.Publishing{
		ContentType:   "text/plain",
		Body:          answer,
		CorrelationId: corId,
	}

	err := c.rabbit.ReplyTo(amqpMsg, replyTo)
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) createNewAnvil(request AnvilRequest, hash string) (chan events.Message, error) {
	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()

	c.log.Debug("before startAnvil")
	anvilID, anvilQueues, err := c.startAnvil(request)

	if ch, ok := c.eventState[anvilID]; ok {
		c.log.Warn("channel for container already exists")
		return ch, nil
	}

	eventsChan := make(chan events.Message, 1)
	c.eventState[anvilID] = eventsChan

	if err == nil {
		c.anvilMutex.Lock()
		c.anvils[anvilID] = &Anvil{
			credsHash: hash,
			queues: Queues{
				rpcQueue:        anvilQueues.rpcQueue,
				publishExchange: anvilQueues.publishExchange,
			},
			status: Starting,
		}
		c.anvilMutex.Unlock()

	}

	return eventsChan, err
}

func (c *Controller) StartAndWait(request AnvilRequest, hash string) error {

	c.log.Debug("Enter StartAndWait")
	_, _, err := c.findAnvil(hash)
	if err == nil {
		c.log.Debug("Anvil found")
		return nil
	}

	c.log.Debug("Before createNewAnvil")

	ch, err := c.createNewAnvil(request, hash)
	if err != nil {
		c.log.Error("Create error: ", err)
		return err
	}

	c.log.Debug("Waiting for healthstatus")
	//ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	c.log.Debug("StartTimeout", c.settings.StartTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.settings.StartTimeout)*time.Minute)
	return c.WaitTillStart(ctx, ch, cancel)
}

func (c *Controller) WaitTillStart(ctx context.Context, events chan events.Message, cancel context.CancelFunc) error {
	for {
		select {
		case msg, ok := <-events:
			if !ok {
				return fmt.Errorf("chan is closed")
			}

			if strings.Contains(msg.Status, "health_status:") {

				c.updateHealthStatus(msg)

				c.eventStateMu.Lock()
				if ch, ok := c.eventState[msg.ID]; ok {
					close(ch)
					delete(c.eventState, msg.ID)
				}

				c.eventStateMu.Unlock()
				cancel()
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("timeout has been reached")
		}
	}
}

func (c *Controller) EventHandler(msg events.Message) {

	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()
	if msg.ID == "" {
		return
	}

	ch, ok := c.eventState[msg.ID]
	if ok {
		ch <- msg
		return
	}

	if strings.Contains(msg.Status, "health_status:") || strings.Contains(msg.Action, "stop") {
		c.updateHealthStatus(msg)
	}

}

func (c *Controller) ErrorHandler(err error) {
	fmt.Println(err)
}
