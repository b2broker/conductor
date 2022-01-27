package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/b2broker/conductor/docker"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/docker/distribution/context"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

func main() {

	///запрос на убийство
	//хранилище состояний SqLite
	//при падении Anvil докер должен перезапускаться (провести эмуляцию )

	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	log := logger.Sugar()

	amqpHost := os.Getenv("AMQP_HOST")
	amqpQueue := os.Getenv("AMQP_QUEUE")
	imgRef := os.Getenv("IMG_PULL")
	dockerLogin := os.Getenv("DOCKER_LOGIN")
	dockerPass := os.Getenv("DOCKER_PASS")
	dockerNetwork := os.Getenv("DOCKER_NETWORK")
	amqpExternal := os.Getenv("AMQP_EXTERNAL_NAME")

	startTimeout, err := strconv.Atoi(os.Getenv("START_TIMEOUT_MINS"))
	if err != nil {
		log.Warn("START_TIMEOUT_MINS is not set. 5 minutes by default")
		startTimeout = 5
	}
	fmt.Println(startTimeout)

	settings := docker.Settings{
		AmqpHost:         amqpHost,
		AuthToken:        docker.AuthSrt(dockerLogin, dockerPass),
		ImgRef:           imgRef,
		NetworkName:      dockerNetwork,
		AmqpExternalName: amqpExternal,
		StartTimeout:     startTimeout,
	}

	//TODO закрытие соединения
	connectRabbitMQ, err := amqp.Dial(amqpHost)
	if err != nil {
		panic(err)
	}

	rabbit, err := rabbitmq.NewRabbit(connectRabbitMQ, amqpQueue, false)
	if err != nil {
		panic(err)
	}

	//создавать коннект в main, и контролировать
	dCtx := context.Background()
	dClient := docker.NewDClient(&dCtx)

	controller := docker.NewController(dClient, rabbit, settings, log)
	controller.RestoreStatus()

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
