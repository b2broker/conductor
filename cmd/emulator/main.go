package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/streadway/amqp"
	"google.golang.org/protobuf/proto"
)

type Anvil struct {
	server   string
	login    uint64
	password string
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

	fmt.Println("published")
}

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

func handler(amqpMsg amqp.Delivery) {

	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	log := logger.Sugar()

	if amqpMsg.CorrelationId == "attach" {
		protoMsg := &conductor.AttachResponse{}
		msg := amqpMsg.Body
		err := proto.Unmarshal(msg, protoMsg)
		if err != nil {
			log.Warn(err)
		}
		log.Warn("Attach response: ", protoMsg)
	} else if amqpMsg.CorrelationId == "detach" {
		protoMsg := &conductor.DetachResponse{}
		msg := amqpMsg.Body
		err := proto.Unmarshal(msg, protoMsg)
		if err != nil {
			log.Warn(err)
		}
		log.Warn("Detach response: ", protoMsg)
	} else {
		fmt.Println("unknown endpoint")
	}

}

func main() {

	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	log := logger.Sugar()

	amqpHost := os.Getenv("AMQP_HOST")
	amqpQueue := os.Getenv("AMQP_QUEUE")

	connectRabbitMQ, err := amqp.Dial(amqpHost)
	if err != nil {
		panic(err)
	}

	rabbit, err := rabbitmq.NewRabbit(connectRabbitMQ, amqpQueue, true)
	if err != nil {
		panic(err)
	}

	go rabbit.Read(handler)

	anvil1 := Anvil{
		server:   "***",
		login:    63,
		password: "***",
	}

	anvil2 := Anvil{
		server:   "***",
		login:    97,
		password: "***",
	}

	log.Warn("Attach anvil1")

	attachRequest(rabbit, anvil1, "attach")
	//
	time.Sleep(130 * time.Second)
	//

	log.Warn("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	time.Sleep(30 * time.Second)

	fmt.Println("Attach anvil1")
	attachRequest(rabbit, anvil1, "attach")
	fmt.Println("Attach anvil2")
	attachRequest(rabbit, anvil2, "attach")

	time.Sleep(130 * time.Second)

	fmt.Println("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	fmt.Println("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	time.Sleep(150 * time.Second)
}
