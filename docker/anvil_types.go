package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/streadway/amqp"
	"google.golang.org/protobuf/proto"
)

type Queues struct {
	rpcQueue        string
	publishExchange string
}

type AnvilStatus string

const (
	Starting  AnvilStatus = "starting"
	Healthy   AnvilStatus = "healthy"
	Unhealthy AnvilStatus = "unhealthy"
	Stopped   AnvilStatus = "stopped"
)

type Anvil struct {
	credsHash string
	queues    Queues
	status    AnvilStatus
}

type AnvilResource struct {
	id     string
	queues Queues
	status AnvilStatus
}

type MetaTraderVersion int

const (
	MT4 MetaTraderVersion = 4
	MT5 MetaTraderVersion = 5
)

type AnvilRequest struct {
	version  MetaTraderVersion
	server   string
	login    uint64
	password string
}

func parseRequest(amqpMsg amqp.Delivery) (AnvilRequest, error) {
	protoMsg := &conductor.ResourceRequest{}
	err := proto.Unmarshal(amqpMsg.Body, protoMsg)
	if err != nil {
		return AnvilRequest{}, err
	}

	var version MetaTraderVersion
	switch protoMsg.ResourceType {
	case conductor.ResourceRequest_METATRADER_4:
		version = MT4
	case conductor.ResourceRequest_METATRADER_5:
		version = MT5
	default:
		return AnvilRequest{}, fmt.Errorf("unknown resource")
	}

	server, ok := protoMsg.Params["server"]
	if !ok {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain server")
	}

	login, ok := protoMsg.Params["login"]
	if !ok {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain login")
	}

	password, ok := protoMsg.Params["password"]
	if !ok {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain password")
	}

	loginValue, err := strconv.ParseUint(login, 10, 64)
	if err != nil {
		return AnvilRequest{}, fmt.Errorf("request contains incorrect login")
	}

	return AnvilRequest{
		version:  version,
		server:   server,
		login:    loginValue,
		password: password,
	}, nil
}

func anvilHash(server string, login uint64, password string) (string, error) {
	hasher := sha256.New()
	if _, err := hasher.Write([]byte(fmt.Sprintf("%s%d%s", server, login, password))); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func prepareStopResponse(errorStr string) ([]byte, error) {
	stopResponse := conductor.DetachResponse{Error: errorStr}
	bytes, err := proto.Marshal(&stopResponse)
	if err == nil {
		return bytes, nil
	}
	return nil, err
}

func (c *Controller) sendStopResponse(errorStr string, corId string, replyTo string) {
	response, err := prepareStopResponse(errorStr)
	if err != nil {
		c.log.Fatal("error while prepare stop response")
		return
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error(err)
		c.log.Fatal("couldn't send stop response")
		return
	}
}

func (c *Controller) sendStartResponse(queues Queues, corId string, replyTo string, err error) {
	errorStr := ""

	if err != nil {
		errorStr = err.Error()
	}

	response, err := prepareCreateResponse(queues, errorStr)
	if err != nil {
		c.log.Error("Can't prepare answer")
		return
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error("Couldn't send answer")
		return
	}
	c.log.Debug("Create Anvil Send Answer")
}

func (c *Controller) sendResourcesResponse(anvils []AnvilResource, corId string, replyTo string) {
	response, err := prepareResourcesResponse(anvils)

	if err != nil {
		c.log.Error("answer can't be prepared")
		return
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error("answer wasn't send")
		return
	}
}

func prepareCreateResponse(anvilQueues Queues, errorStr string) ([]byte, error) {
	channel := conductor.Channel{
		RpcQueue:    []byte(anvilQueues.rpcQueue),
		PubExchange: []byte(anvilQueues.publishExchange),
	}
	createResponse := conductor.AttachResponse{
		Channel: &channel,
		Error:   errorStr,
	}

	bytes, err := proto.Marshal(&createResponse)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func prepareResourcesResponse(anvils []AnvilResource) ([]byte, error) {
	states := conductor.Resources{}

	for _, anvil := range anvils {
		channel := conductor.Channel{
			RpcQueue:    []byte(anvil.queues.rpcQueue),
			PubExchange: []byte(anvil.queues.publishExchange),
		}

		status := conductor.ResourceStatus_STOPPED
		if anvil.status == Starting {
			status = conductor.ResourceStatus_STARTING
		} else if anvil.status == Healthy {
			status = conductor.ResourceStatus_HEALTHY
		}

		resourceStatus := conductor.ResourceStatus{
			Channel: &channel,
			Status:  status,
		}
		states.Statuses = append(states.Statuses, &resourceStatus)
	}

	bytes, err := proto.Marshal(&states)
	if err == nil {
		return bytes, nil

	}
	return nil, err
}
