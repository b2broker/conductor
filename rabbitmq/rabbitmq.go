package rabbitmq

import (
	"fmt"
	"log"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/streadway/amqp"
)

type Rabbit struct {
	connection *rabbitmq.Connection
	channel    *rabbitmq.Channel
	consume    <-chan amqp.Delivery
	Queue      amqp.Queue
}

func NewRabbit(amqpHost string, queueName string, exclusive bool) (*Rabbit, error) {

	rabbitmq.Debug = true
	con, err := rabbitmq.Dial(amqpHost)
	if err != nil {
		log.Panic(err)
	}

	channelRabbitMQ, err := con.Channel()
	if err != nil {
		return nil, err
	}

	qName := ""
	if exclusive == false {
		qName = queueName
	}
	q, err := channelRabbitMQ.QueueDeclare(
		qName,     // name
		true,      // durable
		false,     // delete when unused
		exclusive, // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		panic(err)
	}

	messages, err := channelRabbitMQ.Consume(
		q.Name, // consume name
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no local
		false,  // no wait
		nil,    // arguments
	)
	if err != nil {
		return nil, err
	}

	return &Rabbit{connection: con, channel: channelRabbitMQ, consume: messages, Queue: q}, nil
}

func (r *Rabbit) Stop() {
	r.connection.Close()
	r.channel.Close()
}

func (r *Rabbit) Read(onMsg func(amqp.Delivery)) {
	for {
		for message := range r.consume {
			//message.Ack(false)
			fmt.Println("Get rabbit message")
			onMsg(message)
		}
	}

}

func (r *Rabbit) ExchangeRead(exchangeName string, onMsg func(amqp.Delivery)) error {

	err := r.channel.ExchangeDeclare(
		exchangeName, // name
		"fanout",     // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return err
	}

	err = r.channel.QueueBind(
		r.Queue.Name, // queue name
		"",           // routing key
		exchangeName, // exchange
		false,
		nil,
	)

	if err != nil {
		return err
	}

	msgs, err := r.channel.Consume(
		r.Queue.Name, // queue
		"",           // consumer
		true,         // auto-ack
		false,        // exclusive
		false,        // no-local
		false,        // no-wait
		nil,          // args
	)

	for message := range msgs {
		//message.Ack(false)
		fmt.Println("Get rabbit message  from exchange")
		onMsg(message)
	}

	return nil
}

func (r *Rabbit) ReplyTo(msg amqp.Publishing, replyTo string) error {
	err := r.channel.Publish(
		"",      // exchange
		replyTo, // routing key
		false,   // mandatory
		false,   // immediate
		msg)

	return err
}

func (r *Rabbit) Publish(msg amqp.Publishing) error {
	fmt.Println("Queue name: ", r.Queue.Name)
	err := r.channel.Publish(
		"",       // exchange
		"test_q", // routing key
		false,    // mandatory
		false,    // immediate
		msg)

	return err
}
