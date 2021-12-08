package docker

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

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
}

func NewController(d *DClient, r *rabbitmq.Rabbit, settings Settings) *Controller {
	return &Controller{docker: d, rabbit: r, settings: settings, anvils: make(map[string]Anvil)}
}

func (c *Controller) Handler(msg []byte) error {

	protoMsg := &conductor.Request{}

	err := proto.Unmarshal(msg, protoMsg)
	if err != nil {
		return err
	}

	fmt.Println(protoMsg)

	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s%d%s", protoMsg.Server, protoMsg.Login, protoMsg.Password)))
	hash := hex.EncodeToString(h.Sum(nil))

	if v, ok := c.anvils[hash]; ok {
		fmt.Println("Find anvil")
		c.reply(v.queues)
		return nil
	}

	fmt.Println("Need create new")
	c.createAndReply(protoMsg, hash)

	return nil
}

func (c *Controller) createAnvil(request *conductor.Request) (string, Queues, error) {

	//вынести в поля структуры
	err := c.docker.Pull(c.settings.ImgRef, c.settings.AuthToken)
	if err != nil {
		fmt.Println(err.Error())
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

	err, resID := c.docker.Start(c.settings.ImgRef, env)
	if err != nil {
		fmt.Println(err.Error())
		return "", Queues{}, err
	}

	if c.settings.NetworkName != "" {
		err = c.docker.NetworkConnect(c.settings.NetworkName, resID)
		if err != nil {
			fmt.Println(err.Error())
			return "", Queues{}, err
		}
	}

	queues := Queues{
		rpcQueue:        rpcQueue,
		publishExchange: publishExchange,
	}

	return resID, queues, nil
}

func (c *Controller) reply(anvilQueues Queues) error {
	answer := conductor.Response{
		RpcQueue:     anvilQueues.rpcQueue,
		PublishQueue: anvilQueues.publishExchange,
		Error:        "",
	}

	sAnswer, err := proto.Marshal(&answer)
	if err != nil {
		return err
	}

	err = c.rabbit.Publish(sAnswer)
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) createAndReply(request *conductor.Request, hash string) {
	anvilID, anvilQueues, err := c.createAnvil(request)
	if err != nil {
		//TODO
		return
	}

	c.anvils[anvilID] = Anvil{
		credsHash: hash,
		queues: Queues{
			rpcQueue:        anvilQueues.rpcQueue,
			publishExchange: anvilQueues.publishExchange,
		},
	}

	c.reply(anvilQueues)
}

func (c *Controller) EventHandler(msg events.Message) {
	fmt.Println(msg)
}

func (c *Controller) ErrorHandler(err error) {
	fmt.Println(err)
}
