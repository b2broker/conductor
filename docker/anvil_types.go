package docker

import (
	"crypto/sha1"
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

	msg := amqpMsg.Body
	err := proto.Unmarshal(msg, protoMsg)
	if err != nil {
		return AnvilRequest{}, err
	}

	anvilRequest := AnvilRequest{}

	if protoMsg.ResourceType == conductor.ResourceRequest_METATRADER_4 {
		anvilRequest.version = MT4
	} else if protoMsg.ResourceType == conductor.ResourceRequest_METATRADER_5 {
		anvilRequest.version = MT5
	} else {
		return AnvilRequest{}, fmt.Errorf("unknown recource")
	}

	if v, ok := protoMsg.Params["server"]; ok {
		anvilRequest.server = v
	} else {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain server")
	}

	if v, ok := protoMsg.Params["login"]; ok {
		anvilRequest.login, err = strconv.ParseUint(v, 10, 64)
		if err != nil {
			return AnvilRequest{}, fmt.Errorf("request contains incorrect login")
		}
	} else {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain login")
	}

	if v, ok := protoMsg.Params["password"]; ok {
		anvilRequest.password = v
	} else {
		return AnvilRequest{}, fmt.Errorf("request doesn't contain password")
	}

	return anvilRequest, nil
}

func anvilHash(server string, login uint64, password string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s%d%s", server, login, password)))
	hash := hex.EncodeToString(h.Sum(nil))
	return hash
}

func prepareStopResponse(errorStr string) ([]byte, error) {
	stopResponse := conductor.DetachResponse{Error: errorStr}

	bytes, err := proto.Marshal(&stopResponse)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (c *Controller) sendStopResponse(errorStr string, corId string, replyTo string) {
	response, err := prepareStopResponse(errorStr)
	if err != nil {
		c.log.Error("Error while prepare stop response")
		return
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error("Couldn't send stop response")
		return
	}
	return
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
		c.log.Error("Can't prepare answer")
		return
	}

	err = c.reply(response, corId, replyTo)
	if err != nil {
		c.log.Error("Couldn't send answer")
		return
	}
	c.log.Debug("List Anvil Send Answer")

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

	st := conductor.Resources{}

	for _, anvil := range anvils {

		channel := conductor.Channel{
			RpcQueue:    []byte(anvil.queues.rpcQueue),
			PubExchange: []byte(anvil.queues.publishExchange),
		}
		status := conductor.ResourceStatus_STOPPED
		if anvil.status == Starting {
			status = conductor.ResourceStatus_STARTING
		} else if anvil.status == Healthy {
			status = conductor.ResourceStatus_STARTING
		}
		resourceStatus := conductor.ResourceStatus{
			Channel: &channel,
			Status:  status,
		}
		st.Statuses = append(st.Statuses, &resourceStatus)

	}

	bytes, err := proto.Marshal(&st)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}
