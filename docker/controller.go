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
	rabbit   *rabbitmq.Rabbit
	log      *zap.SugaredLogger
	settings Settings

	anvilMutex sync.RWMutex
	anvils     map[string]*Anvil

	eventStateMu sync.RWMutex
	eventState   map[string]chan events.Message
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

	for id, anvil := range c.anvils {
		if anvil.credsHash == hash {
			return id, anvil, nil
		}
	}

	return "", &Anvil{}, fmt.Errorf("anvil can't be found")
}

func (c *Controller) findOrCreate(request AnvilRequest) (Queues, error) {
	hash, err := anvilHash(request.server, request.login, request.password)
	if err != nil {
		return Queues{}, err
	}

	c.log.Debugf("start the anvil %d@%s", request.login, request.server)

	err = c.StartAndWait(request, hash)
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
	hash, err := anvilHash(request.server, request.login, request.password)
	if err != nil {
		c.log.Error(err)
		return
	}
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

func (c *Controller) processCreate(msg amqp.Delivery) {
	requestedAnvil, err := parseRequest(msg)
	if err != nil {
		c.log.Error(err)
		return
	}
	queues, err := c.findOrCreate(requestedAnvil)
	c.sendStartResponse(queues, msg.CorrelationId, msg.ReplyTo, err)
}

func (c *Controller) processResources(corId string, replyTo string) {

	anvils := c.Status()
	c.sendResourcesResponse(anvils, corId, replyTo)
}

func (c *Controller) Handler(amqpMsg amqp.Delivery) {
	c.log.Debugw(
		"got request",
		"Corr: ", amqpMsg.CorrelationId,
		"ReplyTo: ", amqpMsg.ReplyTo,
	)

	path, ok := amqpMsg.Headers["endpoint"]
	if !ok {
		return
	}

	c.log.Debug("endpoint: ", path)
	switch path {
	case "ConductorService.Attach":
		go c.processCreate(amqpMsg)
	case "ConductorService.Detach":
		requestedAnvil, err := parseRequest(amqpMsg)
		if err == nil {
			c.processStop(requestedAnvil, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
		}
	case "ConductorService.List":
		c.processResources(amqpMsg.CorrelationId, amqpMsg.ReplyTo)
	default:
		c.log.Debug("requested endpoint not served")
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
	rpcQueue := uuid.New().String()
	pubExchange := uuid.New().String()

	envs := []string{
		fmt.Sprintf("MT_ADDRESS=%s", request.server),
		fmt.Sprintf("MT_LOGIN=%d", request.login),
		fmt.Sprintf("MT_PASSWORD=%s", request.password),
		// TODO: should be fixed, not dsn but HOST required
		fmt.Sprintf("AMQP_HOST=%s", c.settings.AmqpExternalName),
		fmt.Sprintf("AMQP_RPC_QUEUE=%s", rpcQueue),
		fmt.Sprintf("AMQP_PUBLISH_EXCHANGE=%s", pubExchange),
	}

	id, err := c.docker.Create(c.settings.ImgRef, envs)
	if err != nil {
		c.log.Errorw("can't create container", "error", err)
		return "", Queues{}, err
	}

	_, err = c.docker.Start(id, c.settings.NetworkName)
	if err != nil {
		c.log.Errorw("can't start container", "error", err)
		return "", Queues{}, err
	}

	queues := Queues{
		rpcQueue:        rpcQueue,
		publishExchange: pubExchange,
	}

	return id, queues, nil
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
	id, queues, err := c.startAnvil(request)

	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()
	if ch, ok := c.eventState[id]; ok {
		c.log.Warn("channel for container already exists")
		return ch, nil
	}

	events := make(chan events.Message, 1)
	c.eventState[id] = events

	if err == nil {
		c.anvilMutex.Lock()
		c.anvils[id] = &Anvil{
			credsHash: hash,
			queues: Queues{
				rpcQueue:        queues.rpcQueue,
				publishExchange: queues.publishExchange,
			},
			status: Starting,
		}
		c.anvilMutex.Unlock()
	}

	return events, err
}

func (c *Controller) StartAndWait(request AnvilRequest, hash string) (string, *Anvil, error) {
	id, anvil, err := c.findAnvil(hash)
	if err == nil {
		// TODO: probably it is not in a running stance
		return id, anvil, nil
	}

	// TODO: check status
	ch, err := c.createNewAnvil(request, hash)
	if err != nil {
		c.log.Error("anvil error on creation: ", err)
		return "", nil, err
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

			if strings.Contains(msg.Status, "health_status:") ||
				strings.Contains(msg.Action, "die") {

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
	if msg.ID == "" {
		return
	}

	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()
	ch, ok := c.eventState[msg.ID]
	if ok {
		ch <- msg
		return
	}
	// TODO: When this will be triggered?
	// In case not ok previous — means that we don't have such element in the map
	if strings.Contains(msg.Status, "health_status:") ||
		strings.Contains(msg.Action, "stop") ||
		strings.Contains(msg.Action, "die") {
		c.updateHealthStatus(msg)
	}
}

func (c *Controller) ErrorHandler(err error) {
	fmt.Println(err)
}

func (c *Controller) Status() []AnvilResource {
	c.anvilMutex.RLock()
	defer c.anvilMutex.RUnlock()

	var anvils []AnvilResource
	for dockerId, anvil := range c.anvils {
		resource := AnvilResource{
			id:     dockerId,
			queues: anvil.queues,
			status: anvil.status,
		}
		anvils = append(anvils, resource)
	}

	return anvils

}
