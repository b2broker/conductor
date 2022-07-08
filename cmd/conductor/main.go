package main

import (
	"encoding/json"
	"log"

	"github.com/b2broker/conductor/docker"
	"github.com/b2broker/conductor/options"
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
		log.Panicln(err)
	}

	logger, err := cfg.Build()
	if err != nil {
		log.Panicln(err)
	}

	defer logger.Sync() // flushes buffer, if any
	log := logger.Sugar()

	var (
		amqpDsn        string = "amqp://guest:guest@127.0.0.1:5672" // DSN record
		startupTimeout int64  = 5                                   // In seconds
		rpcQueue       string = "conductor"                         // RPC queue to serve on
	)

	opts, err := options.Configure([]options.Option{
		options.AmqpDSN("AMQP_DSN", amqpDsn),
		options.RegistryLogin("REGISTRY_LOGIN"),
		options.RegistryPassword("REGISTRY_PASS"),
		options.RPCQueue("CONDUCTOR_RPC_QUEUE", rpcQueue),
		options.StartupTimeout("STARTUP_TIMEOUT", startupTimeout),
		options.AttachableNetwork("ATTACH_NETWORK"),
		// TODO: should be removed
		options.DockerImage("DOCKER_IMAGE"),
	}...)

	if err != nil {
		log.Panic(err)
	}

	settings := docker.Settings{
		AmqpHost: opts.AmqpDSN().String(),
		AuthToken: docker.AuthSrt(
			opts.RegistryLogin(),
			opts.RegistryPassword(),
		),
		StartTimeout: int(opts.StartupTimeout().Minutes()),
		NetworkName:  opts.AttachableNetwork(),
		// TODO: should be removed
		AmqpExternalName: opts.AmqpDSN().Hostname(),
		ImgRef:           opts.DockerImage(),
	}

	rabbit, err := rabbitmq.NewRabbit(
		opts.AmqpDSN().String(),
		opts.RPCQueue(),
	)
	if err != nil {
		log.Panic(err)
	}

	dockerCli, err := docker.NewClient(context.Background())
	if err != nil {
		log.Panic(err)
	}
	// original dockerCli have Close method

	controller, err := docker.NewController(dockerCli, rabbit, settings, log)
	if err != nil {
		log.Panic(err)
	}

	go rabbit.Read(controller.Handler)

	dockerCli.Events(controller.EventHandler, controller.ErrorHandler)

	//var wg sync.WaitGroup
	//wg.Add(1)
	//go func() {
	//	dClient.Events(controller.EventHandler, controller.ErrorHandler)
	//	wg.Done()
	//}()
	//
	//wg.Wait()

	//	отслеживать сигнал, graceful shutdown

}
