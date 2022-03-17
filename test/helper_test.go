package test

import (
	"fmt"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/streadway/amqp"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"google.golang.org/protobuf/proto"
)

func attachRequest(r *rabbitmq.Rabbit, anvil Anvil, corId string) {

	paramMap := make(map[string]string)
	paramMap["server"] = anvil.server
	paramMap["login"] = strconv.FormatUint(anvil.login, 10)
	paramMap["password"] = anvil.password

	msg := conductor.ResourceRequest{
		ResourceType: conductor.ResourceRequest_METATRADER_4,
		Params:       paramMap,
	}

	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	sender(r, s, corId, "ConductorService.Attach")
}

func detachRequest(r *rabbitmq.Rabbit, anvil Anvil, corId string) {

	paramMap := make(map[string]string)
	paramMap["server"] = anvil.server
	paramMap["login"] = strconv.FormatUint(anvil.login, 10)
	paramMap["password"] = anvil.password

	msg := conductor.ResourceRequest{
		ResourceType: conductor.ResourceRequest_METATRADER_4,
		Params:       paramMap,
	}

	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	sender(r, s, corId, "ConductorService.Detach")

}

func statusRequest(r *rabbitmq.Rabbit, corId string) {
	msg := empty.Empty{}

	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}
	sender(r, s, corId, "ConductorService.List")
}

func sender(r *rabbitmq.Rabbit, msg []byte, corId string, endpoint string) {

	header := make(map[string]interface{})
	header["endpoint"] = endpoint

	amqpMsg := amqp.Publishing{
		Headers:       header,
		ContentType:   "text/plain",
		Body:          msg,
		CorrelationId: corId,
		ReplyTo:       r.Queue.Name,
	}
	err := r.Publish(amqpMsg)
	if err != nil {
		fmt.Println("Can't publish request")
		return
	}

	//fmt.Println("published")
}

func parseAttachResponse(msg amqp.Delivery, corrId string) (*conductor.AttachResponse, error) {

	protoMsg := &conductor.AttachResponse{}
	if msg.CorrelationId == corrId {
		err := proto.Unmarshal(msg.Body, protoMsg)
		if err != nil {
			return &conductor.AttachResponse{}, err
		}
	}

	log.Debug("Attach response: ", &protoMsg)
	return protoMsg, nil
}

func parseStatusResponse(msg amqp.Delivery, corrId string) (*conductor.Resources, error) {
	protoMsg := &conductor.Resources{}

	if msg.CorrelationId == corrId {
		err := proto.Unmarshal(msg.Body, protoMsg)
		if err != nil {
			return protoMsg, err
		}
	}
	return protoMsg, nil
}

func parseDetachResponse(msg amqp.Delivery, corrId string) (*conductor.DetachResponse, error) {

	protoMsg := &conductor.DetachResponse{}
	if msg.CorrelationId == corrId {
		err := proto.Unmarshal(msg.Body, protoMsg)
		if err != nil {
			return protoMsg, err
		}
	}

	return protoMsg, nil
}
