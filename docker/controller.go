package docker

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"

	"github.com/streadway/amqp"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/docker/docker/api/types/events"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
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

type Anvil struct {
	credsHash string
	queues    Queues
}

type Controller struct {
	docker   *DClient
	anvils   map[string]Anvil
	rabbit   *rabbitmq.Rabbit
	settings Settings
	slog     *zap.SugaredLogger
}

func NewController(d *DClient, r *rabbitmq.Rabbit, settings Settings, logger *zap.SugaredLogger) *Controller {
	return &Controller{docker: d, rabbit: r, settings: settings, anvils: make(map[string]Anvil), slog: logger}
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

func anvilHash(request *conductor.Request) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s%d%s", request.Server, request.Login, request.Password)))
	hash := hex.EncodeToString(h.Sum(nil))
	return hash
}

func (c *Controller) findAnvil(hash string) (string, Anvil, error) {

	for dockerId, anvil := range c.anvils {
		if anvil.credsHash == hash {
			return dockerId, anvil, nil
		}
	}

	return "", Anvil{}, fmt.Errorf("can't find Anvil")

}

func (c *Controller) processCreate(request *conductor.Request, corId string, replyTo string) error {

	hash := anvilHash(request)
	_, anvil, err := c.findAnvil(hash)
	if err != nil {
		c.slog.Debug("Create new Anvil")
		return c.createAndReply(request, hash, corId, replyTo)
	}

	c.slog.Debug("Anvil exist, only reply")

	response, err := prepareCreateResponse(anvil.queues, "")
	if err != nil {
		return err
	}
	return c.reply(response, corId, replyTo)
}

func (c *Controller) processStop(request *conductor.Request, corId string, replyTo string) error {
	hash := anvilHash(request)
	id, anvil, err := c.findAnvil(hash)
	if err != nil {
		c.slog.Debug("Try to stop Anvil. Such Anvil doesn't not exist")
		response, err := prepareStopResponse(err.Error())
		if err != nil {
			return err
		}
		return c.reply(response, corId, replyTo)
	}
	c.slog.Debug("Need to Stop Anvil", anvil.credsHash)

	errSrt := ""

	err = c.docker.Stop(id)
	if err != nil {
		errSrt = err.Error()
	} else {
		//delete only if stop
		delete(c.anvils, id)
	}

	response, err := prepareStopResponse(errSrt)
	if err != nil {
		return err
	}

	return c.reply(response, corId, replyTo)

}

func (c *Controller) Handler(amqpMsg amqp.Delivery) error {

	c.slog.Debugw("Get request",
		"CorId", amqpMsg.CorrelationId,
		"ReplyTo", amqpMsg.ReplyTo)

	if v, k := amqpMsg.Headers["endpoint"]; k {
		c.slog.Debug("Endpoint", v)
		if v == "Create" {
			request, err := parseCreateRequest(amqpMsg)
			if err == nil {
				return c.processCreate(request, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
			}
		} else if v == "Stop" {
			request, err := parseStopRequest(amqpMsg)
			if err == nil {
				return c.processStop(request, amqpMsg.CorrelationId, amqpMsg.ReplyTo)
			}
		} else {
			return fmt.Errorf("unknown command")
		}
	}

	return fmt.Errorf("no endpoint")
}

func (c *Controller) createAnvil(request *conductor.Request) (string, Queues, error) {
	err := c.docker.Pull(c.settings.ImgRef, c.settings.AuthToken)
	if err != nil {
		c.slog.Errorw("Can't pull image", "error", err.Error())
		return "", Queues{}, err
	}

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
		c.slog.Errorw("Can't start container", "error", err.Error())
		return "", Queues{}, err
	}

	queues := Queues{
		rpcQueue:        rpcQueue,
		publishExchange: publishExchange,
	}

	return resID, queues, nil
}

func prepareStopResponse(errorStr string) ([]byte, error) {
	stopResponse := conductor.StopResponse{Error: errorStr}

	bytes, err := proto.Marshal(&stopResponse)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func prepareCreateResponse(anvilQueues Queues, errorStr string) ([]byte, error) {
	createResponse := conductor.CreateResponse{
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

func (c *Controller) createAndReply(request *conductor.Request, hash string, corId string, replyTo string) error {
	anvilID, anvilQueues, err := c.createAnvil(request)

	errSrt := ""
	if err == nil {
		fmt.Println("err == nil")
		c.anvils[anvilID] = Anvil{
			credsHash: hash,
			queues: Queues{
				rpcQueue:        anvilQueues.rpcQueue,
				publishExchange: anvilQueues.publishExchange,
			},
		}

	} else {
		errSrt = err.Error()
	}

	response, err := prepareCreateResponse(anvilQueues, errSrt)
	if err != nil {
		return err
	}

	return c.reply(response, corId, replyTo)

}

func (c *Controller) EventHandler(msg events.Message) {
	fmt.Println(msg)
}

func (c *Controller) ErrorHandler(err error) {
	fmt.Println(err)
}
