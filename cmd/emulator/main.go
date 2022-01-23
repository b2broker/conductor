package main

import (
	"fmt"
	"os"
	"time"

	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/streadway/amqp"
	"google.golang.org/protobuf/proto"
)

type Anvil struct {
	server   string
	login    int64
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

	//msg := conductor.Request{
	//	Server:   "***",
	//	Login:    0,
	//	Password: "**",
	//}
	msg := conductor.Request{
		Server:   anvil.server,
		Login:    anvil.login,
		Password: anvil.password,
	}

	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	sender(r, s, corId, "ConductorService.Attach")
}

func detachRequest(r *rabbitmq.Rabbit, anvil Anvil, corId string) {

	msg := conductor.Request{
		Server:   anvil.server,
		Login:    anvil.login,
		Password: anvil.password,
	}

	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	sender(r, s, corId, "ConductorService.Detach")

}

func handler(amqpMsg amqp.Delivery) {

	if amqpMsg.CorrelationId == "attach" {
		protoMsg := &conductor.AttachResponse{}
		msg := amqpMsg.Body
		err := proto.Unmarshal(msg, protoMsg)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("Attach response: ", protoMsg)
	} else if amqpMsg.CorrelationId == "detach" {
		protoMsg := &conductor.DetachResponse{}
		msg := amqpMsg.Body
		err := proto.Unmarshal(msg, protoMsg)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("Detach response: ", protoMsg)
	} else {
		fmt.Println("unknown endpoint")
	}

}

func main() {
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
		login:    0,
		password: "***",
	}

	anvil2 := Anvil{
		server:   "***",
		login:    2,
		password: "***",
	}

	fmt.Println("Attach anvil1")
	attachRequest(rabbit, anvil1, "attach")
	//
	time.Sleep(70 * time.Second)
	//
	fmt.Println("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	time.Sleep(10 * time.Second)

	fmt.Println("Attach anvil1")
	attachRequest(rabbit, anvil1, "attach")
	fmt.Println("Attach anvil2")
	attachRequest(rabbit, anvil2, "attach")

	time.Sleep(70 * time.Second)

	fmt.Println("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	fmt.Println("Detach anvil1")
	detachRequest(rabbit, anvil1, "detach")
	time.Sleep(20 * time.Second)
}
