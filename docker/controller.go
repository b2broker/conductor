package docker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/docker/docker/api/types/events"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

type Settings struct {
	AmqpHost    string
	AuthToken   string
	ImgRef      string
	NetworkName string
	LifeTime    int32
}

type Queues struct {
	rpcQueue        string
	publishExchange string
}

type AnvilStatus string

const (
	Starting AnvilStatus = "starting"
	Healthy  AnvilStatus = "healthy"
	Dead     AnvilStatus = "dead"
)

type Anvil struct {
	credsHash string
	queues    Queues
	status    AnvilStatus
}

type Controller struct {
	docker   *DClient
	anvils   map[string]*Anvil
	rabbit   *rabbitmq.Rabbit
	settings Settings
	log      *zap.SugaredLogger

	eventState   map[string]chan events.Message
	eventStateMu sync.RWMutex
	anvilMutex   sync.RWMutex
}

func NewController(d *DClient, r *rabbitmq.Rabbit, settings Settings, logger *zap.SugaredLogger) *Controller {
	return &Controller{
		docker:     d,
		rabbit:     r,
		settings:   settings,
		anvils:     make(map[string]*Anvil),
		log:        logger,
		eventState: make(map[string]chan events.Message),
	}
}

func parseCreateRequest(amqpMsg amqp.Delivery) (*conductor.Request, error) {
	protoMsg := &conductor.Request{}

	msg := amqpMsg.Body
	err := proto.Unmarshal(msg, protoMsg)
	if err != nil {
		return nil, err
	}
	return protoMsg, nil
}

func parseStopRequest(amqpMsg amqp.Delivery) (*conductor.Request, error) {
	protoMsg := &conductor.Request{}

	msg := amqpMsg.Body
	err := proto.Unmarshal(msg, protoMsg)
	if err != nil {
		return nil, err
	}
	return protoMsg, nil
}

func anvilHash(server string, login int64, password string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s%d%s", server, login, password)))
	hash := hex.EncodeToString(h.Sum(nil))
	return hash
}

func (c *Controller) findAnvil(hash string) (string, *Anvil, error) {

	c.anvilMutex.RLock()
	defer c.anvilMutex.RUnlock()

	for dockerId, anvil := range c.anvils {
		if anvil.credsHash == hash {
			fmt.Println("!!!Find anvil")
			return dockerId, anvil, nil
		}
	}

	return "", &Anvil{}, fmt.Errorf("can't find Anvil")

}

func (c *Controller) findOrCreate(request *conductor.Request) (error, Queues) {

	hash := anvilHash(request.Server, request.Login, request.Password)
	fmt.Println("New server: ", request.Server, "login: ", request.Login, "pass: ", request.Password)
	fmt.Println("Create new with hash:", hash)

	err := c.StartAndWait(request, hash)
	if err != nil {
		fmt.Println("Can't create new Anvil")
		return err, Queues{}
	}

	_, anvil, err := c.findAnvil(hash)

	if err != nil {
		fmt.Println("!!!!!")
		return err, Queues{}
	}

	if anvil.status != Healthy {
		fmt.Println("container unhealthy")
		return fmt.Errorf("container unhealthy"), Queues{}
	}

	return nil, anvil.queues
}

func (c *Controller) processStop(request *conductor.Request, corId string, replyTo string) {
	hash := anvilHash(request.Server, request.Login, request.Password)
	id, anvil, err := c.findAnvil(hash)
	if err != nil {
		c.log.Debug("Try to stop Anvil. Such Anvil doesn't not exist")
		response, err := prepareStopResponse(err.Error())
		if err != nil {
			c.log.Error("Can't parse stop request")
			return
		}
		err = c.reply(response, corId, replyTo)
		if err != nil {
			c.log.Error("Can't send stop answer")
		}
	}
	c.log.Debug("Need to Stop Anvil", anvil.credsHash)

	errSrt := ""

	err = c.docker.Stop(id)
	if err != nil {
		errSrt = err.Error()
	} else {
		//delete only if stop
		c.anvilMutex.Lock()
		delete(c.anvils, id)
		c.anvilMutex.Unlock()
	}

	response, err := prepareStopResponse(errSrt)
	if err != nil {
		c.log.Error("Can't prepare stop answer")
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error("Can't send stop answer")
	}

}

func (c *Controller) processCreate(amqpMsg amqp.Delivery) {

	request, err := parseCreateRequest(amqpMsg)
	if err != nil {
		c.log.Error("Can't parse msg")
		return
	}

	err, queues := c.findOrCreate(request)
	createErr := ""
	if err != nil {
		createErr = err.Error()
	}

	response, err := prepareCreateResponse(queues, createErr)
	if err != nil {
		c.log.Error("Can't prepare answer")
		return
	}

	err = c.reply(response, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
	if err != nil {
		c.log.Error("Can't send answer")
		return
	}
	fmt.Println("Create Anvil Send Answer")
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
			request, err := parseStopRequest(amqpMsg)
			if err == nil {
				c.processStop(request, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
			}
		}
	}
}

func (c *Controller) startAnvil(request *conductor.Request) (string, Queues, error) {
	//err := c.docker.Pull(c.settings.ImgRef, c.settings.AuthToken)
	//if err != nil {
	//	c.log.Errorw("Can't pull image", "error", err.Error())
	//	return "", Queues{}, err
	//}

	fmt.Println("startAnvil")
	rpcQueue := uuid.New().String()
	publishExchange := uuid.New().String()

	env := []string{fmt.Sprintf("MT_ADDRESS=%s", request.Server),
		fmt.Sprintf("MT_LOGIN=%d", request.Login),
		fmt.Sprintf("MT_PASSWORD=%s", request.Password),
		//fmt.Sprintf("AMQP_HOST=%s", strings.Replace(c.settings.AmqpHost, "amqp://", "", -1)),
		fmt.Sprintf("AMQP_HOST=%s", "rabbitmq"), //TODO
		fmt.Sprintf("AMQP_RPC_QUEUE=%s", rpcQueue),
		fmt.Sprintf("AMQP_PUBLISH_EXCHANGE=%s", publishExchange)}

	err, resID := c.docker.Start(c.settings.ImgRef, env, c.settings.NetworkName)
	if err != nil {
		c.log.Errorw("Can't start container", "error", err.Error())
		return "", Queues{}, err
	}

	queues := Queues{
		rpcQueue:        rpcQueue,
		publishExchange: publishExchange,
	}

	return resID, queues, nil
}

func prepareStopResponse(errorStr string) ([]byte, error) {
	stopResponse := conductor.DetachResponse{Error: errorStr}

	bytes, err := proto.Marshal(&stopResponse)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func prepareCreateResponse(anvilQueues Queues, errorStr string) ([]byte, error) {
	createResponse := conductor.AttachResponse{
		RpcQueue:     anvilQueues.rpcQueue,
		PublishQueue: anvilQueues.publishExchange,
		Error:        errorStr,
	}

	bytes, err := proto.Marshal(&createResponse)
	if err != nil {
		return nil, err
	}

	return bytes, nil
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

func (c *Controller) createNewAnvil(request *conductor.Request, hash string) (chan events.Message, error) {
	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()

	fmt.Println("before startAnvil")
	anvilID, anvilQueues, err := c.startAnvil(request)
	// TODO: check already existed
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

func (c *Controller) StartAndWait(request *conductor.Request, hash string) error {

	_, _, err := c.findAnvil(hash)
	if err == nil {
		return nil
	}

	ch, err := c.createNewAnvil(request, hash)
	if err != nil {
		return err
	}

	// TODO: handle cancel
	ctx, _ := context.WithTimeout(context.Background(), time.Second*120)
	return c.WaitTillStart(ctx, ch)
}

func (c *Controller) WaitTillStart(ctx context.Context, events chan events.Message) error {
	for {
		select {
		case msg, ok := <-events:
			if !ok {
				return fmt.Errorf("chan is closed")
			}

			//fmt.Println("Receive msg from:", msg.ID)

			//fmt.Println("ID:", msg.ID)
			//fmt.Println("Type:", msg.Type)
			//fmt.Println("Actor:", msg.Actor)
			//fmt.Println("Action:", msg.Action)
			//fmt.Println("From:", msg.From)
			//fmt.Println("Scope:", msg.Scope)
			//fmt.Println("status:", msg.Status)
			//fmt.Println("Time:", msg.Time)

			if strings.Contains(msg.Status, "health_status:") {
				c.anvilMutex.Lock()

				if anv, ok := c.anvils[msg.ID]; ok {
					if strings.Contains(msg.Status, "healthy") {
						anv.status = Healthy
						fmt.Println("Now container is healthy and can reply")
					} else if strings.Contains(msg.Status, "unhealthy") {
						anv.status = Dead
						fmt.Println("Now container is unhealthy and can reply")
					}

					c.anvilMutex.Unlock()

					c.eventStateMu.Lock()
					if ch, ok := c.eventState[msg.ID]; ok {
						close(ch)
						delete(c.eventState, msg.ID)
					}

					c.eventStateMu.Unlock()
					return nil
				}

			}
		case <-ctx.Done():
			return fmt.Errorf("timeout has been reached")
		}
	}
}

func (c *Controller) EventHandler(msg events.Message) {
	//fmt.Printf("GOT NEW EVENT: %+v\n", msg)

	c.eventStateMu.Lock()
	defer c.eventStateMu.Unlock()
	if msg.ID == "" {
		return
	}

	//fmt.Println("Sending event to:", msg.ID)

	ch, ok := c.eventState[msg.ID]
	if !ok {
		return
	}

	//fmt.Println("Send events to channel", msg.ID)
	ch <- msg

}

func (c *Controller) ErrorHandler(err error) {
	fmt.Println(err)
}

func (c *Controller) RestoreStatus() {
	containersConfig := c.docker.ReadEnv()

	for k, v := range containersConfig {

		if strings.Contains(v.Image, "dolt") {

			envs := make(map[string]string)

			for _, item := range v.Env {
				parts := strings.Split(item, "=")
				if len(parts) != 2 {
					continue
				}
				key := parts[0]
				value := parts[1]

				envs[key] = value
			}

			address := ""
			login := int64(0)
			password := ""

			publicExchange := ""
			rpcQueue := ""

			//TODO ok != nill
			if v, ok := envs["AMQP_PUBLISH_EXCHANGE"]; ok {
				publicExchange = v
			}
			if v, ok := envs["AMQP_RPC_QUEUE"]; ok {
				rpcQueue = v
			}
			if v, ok := envs["MT_ADDRESS"]; ok {
				address = v
			}
			if v, ok := envs["MT_LOGIN"]; ok {
				login, _ = strconv.ParseInt(v, 10, 64)
			}
			if v, ok := envs["MT_PASSWORD"]; ok {
				password = v
			}

			anvilHash := anvilHash(address, login, password)

			hl, err := c.docker.HealthStatus(k)
			if err != nil {
				c.log.Error("Can't get health status of container: ", k)
				continue
			}
			status := Dead
			if hl == "healthy" {
				status = Healthy
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
			c.anvils[k] = &anvil
			c.anvilMutex.Unlock()
			fmt.Println("Add anvil with hash:", anvilHash)

		}

	}

}
