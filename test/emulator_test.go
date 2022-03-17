package test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/b2broker/conductor/rabbitmq"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type Anvil struct {
	server   string
	login    uint64
	password string
}

func handler(amqpMsg amqp.Delivery) {
	rChan <- amqpMsg
}

var (
	anvils []Anvil
	rabbit *rabbitmq.Rabbit
	log    *zap.SugaredLogger
	rChan  chan amqp.Delivery
)

func TestMain(m *testing.M) {

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
	log = logger.Sugar()

	//AMQP_HOST=amqp://172.19.0.2;
	//// AMQP_QUEUE=test_q
	//amqpHost := os.Getenv("AMQP_HOST")
	//amqpQueue := os.Getenv("AMQP_QUEUE")

	amqpHost := "amqp://172.19.0.2"
	amqpQueue := "test_q"
	fmt.Println("host", amqpHost)

	rabbit, err = rabbitmq.NewRabbit(amqpHost, amqpQueue, true)
	if err != nil {
		panic(err)
	}

	anvils = []Anvil{
		{
			server:   "****",
			login:    50,
			password: "***",
		},
		{
			server:   "****",
			login:    97,
			password: "***",
		},
		{
			server:   "****",
			login:    97,
			password: "45678ffh",
		},
	}

	rChan = make(chan amqp.Delivery)
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestCase1(t *testing.T) {
	go rabbit.Read(handler)

	log.Warn("Attach anvil1")
	corrId := "attach"
	attachRequest(rabbit, anvils[0], corrId)
	msg := <-rChan

	attachResponse, err := parseAttachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, attachResponse.Error, "")

	corrId = "list"
	statusRequest(rabbit, corrId)
	msg = <-rChan
	statusResponse, err := parseStatusResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, 1, len(statusResponse.Statuses), "")
	assert.Equal(t, "HEALTHY", statusResponse.Statuses[0].Status.String(), "")

	log.Warn("Detach anvil1")
	corrId = "detach"
	detachRequest(rabbit, anvils[0], corrId)

	msg = <-rChan
	detachResponse, err := parseDetachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, "", detachResponse.Error)

	corrId = "list"
	statusRequest(rabbit, corrId)
	msg = <-rChan
	statusResponse, err = parseStatusResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, 0, len(statusResponse.Statuses), "")
}

func TestCase2(t *testing.T) {
	go rabbit.Read(handler)

	log.Warn("Attach anvil1")
	corrId := "attach"
	attachRequest(rabbit, anvils[0], corrId)

	time.Sleep(time.Second * 10)
	corrId = "list"
	statusRequest(rabbit, corrId)

	msg := <-rChan
	statusResponse, err := parseStatusResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, 1, len(statusResponse.Statuses), "")
	assert.Equal(t, "STARTING", statusResponse.Statuses[0].Status.String(), "")

	msg = <-rChan

	attachResponse, err := parseAttachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, attachResponse.Error, "")

	log.Warn("Detach anvil1")
	corrId = "detach"
	detachRequest(rabbit, anvils[0], corrId)

	msg = <-rChan
	detachResponse, err := parseDetachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, "", detachResponse.Error)
}

func TestCase3Invalid(t *testing.T) {
	go rabbit.Read(handler)

	log.Warn("Attach anvil Invalid")
	corrId := "attach"
	attachRequest(rabbit, anvils[2], corrId)

	time.Sleep(time.Second * 20)

	corrId = "list"
	statusRequest(rabbit, corrId)
	msg := <-rChan
	statusResponse, err := parseStatusResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, 1, len(statusResponse.Statuses), "")
	assert.Equal(t, "STARTING", statusResponse.Statuses[0].Status.String(), "")

	msg = <-rChan

	attachResponse, err := parseAttachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	//fmt.Println("Attach response:", &attachResponse.Error)
	fmt.Println("Attach response:", attachResponse.Channel.PubExchange)
	fmt.Println("Attach response:", attachResponse.Channel.RpcQueue)
	assert.NotEmpty(t, attachResponse.Error)

	log.Warn("Detach anvil1")
	corrId = "detach"
	detachRequest(rabbit, anvils[0], corrId)

	msg = <-rChan
	detachResponse, err := parseDetachResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, "", detachResponse.Error)

	corrId = "list"
	statusRequest(rabbit, corrId)
	msg = <-rChan
	statusResponse, err = parseStatusResponse(msg, corrId)
	if err != nil {
		t.Error("Couldn't parse msg", err)
	}
	assert.Equal(t, 0, len(statusResponse.Statuses), "")
}
