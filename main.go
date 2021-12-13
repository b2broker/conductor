package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/docker/distribution/context"

	"github.com/b2broker/conductor/docker"
	"github.com/b2broker/conductor/proto/src_out/conductor"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/golang/protobuf/proto"
	"github.com/streadway/amqp"
)

func sender() {

	conn, err := amqp.Dial("amqp://172.19.0.2")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"test_q", // name
		false,    // durable
		false,    // delete when unused
		false,    // exclusive
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		panic(err)
	}

	msg := conductor.Request{
		Server:   "*",
		Login:    *,
		Password: "*",
	}
	s, err := proto.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	err = ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        s,
		})

	if err != nil {
		panic(err)
	}
}

// key-value для хранения состояния
// либо костыль убивать все контейнеры при старке

func main() {

	sender()

	//все значения в env
	//amqpHost := "amqp://172.19.0.2"
	//rQueue := "test_q"
	//imgRef := "vcshl.b2broker.tech:5050/tor/anvil/anvil_art/anvil:2.0.0"

	amqpHost := os.Getenv("AMQP_HOST")
	amqpQueue := os.Getenv("AMQP_QUEUE")
	imgRef := os.Getenv("IMG_PULL")
	dockerLogin := os.Getenv("DOCKER_LOGIN")
	dockerPass := os.Getenv("DOCKER_PASS")
	dockerNetwork := os.Getenv("DOCKER_NETWORK")

	fmt.Println("!!!", amqpQueue)

	settings := docker.Settings{
		AmqpHost:    amqpHost,
		AuthToken:   docker.AuthSrt(dockerLogin, dockerPass),
		ImgRef:      imgRef,
		NetworkName: dockerNetwork,
	}

	connectRabbitMQ, err := amqp.Dial(amqpHost)
	if err != nil {
		panic(err)
	}

	rabbit, err := rabbitmq.NewRabbit(connectRabbitMQ, amqpQueue)
	if err != nil {
		panic(err)
	}

	//создавать коннект в main, и контролировать
	dCtx := context.Background()
	dClient := docker.NewDClient(&dCtx)

	//только rabbit
	controller := docker.NewController(dClient, rabbit, settings)

	go rabbit.Read(controller.Handler)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		dClient.Events(controller.EventHandler, controller.ErrorHandler)
		wg.Done()
	}()

	wg.Wait()

	//	отслеживать сигнал, graceful shutdown

}
