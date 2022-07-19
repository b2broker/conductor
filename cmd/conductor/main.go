package main

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/b2broker/conductor/docker"
	"github.com/b2broker/conductor/rabbitmq"
	"github.com/docker/distribution/context"
	"go.uber.org/zap"
)

func main() {
	rawJSON := []byte(`{
	"level": "debug",
	"encoding": "json",
	"outputPaths": ["stdout"],
	"errorOutputPaths": ["stderr"],
	"encoderConfig": {
		"messageKey": "message",
		"levelKey": "level",
		"levelEncoder": "lowercase"
	}
}`)
	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
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

	settings := docker.Settings{
		AmqpHost:         amqpHost,
		AuthToken:        docker.AuthSrt(dockerLogin, dockerPass),
		ImgRef:           imgRef,
		NetworkName:      dockerNetwork,
		AmqpExternalName: amqpExternal,
		StartTimeout:     startTimeout,
	}

	rabbit, err := rabbitmq.NewRabbit(amqpHost, amqpQueue, false)
	if err != nil {
		log.Panicf("could not connect to rabbit: %v", err)
	}

	//создавать коннект в main, и контролировать
	dClient := docker.NewClient(context.Background())

	controller := docker.NewController(dClient, rabbit, settings, log)
	controller.RestoreStatus()

	go rabbit.Read(controller.Handler)

	dClient.Events(controller.EventHandler, controller.ErrorHandler, []string{imgRef})
}
