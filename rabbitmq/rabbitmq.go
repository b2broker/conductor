package rabbitmq

import (
	"log"

	"github.com/streadway/amqp"
)

type Rabbit struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	consume    <-chan amqp.Delivery
	queue      amqp.Queue
}

func NewRabbit(con *amqp.Connection, queueName string) (*Rabbit, error) {

	channelRabbitMQ, err := con.Channel()
	if err != nil {
		return nil, err
	}

	q, err := channelRabbitMQ.QueueDeclare(
		queueName, // name
		false,     // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		panic(err)
	}

	messages, err := channelRabbitMQ.Consume(
		queueName, // consume name
		"",        // consumer
		true,      // auto-ack
		false,     // exclusive
		false,     // no local
		false,     // no wait
		nil,       // arguments
	)
	if err != nil {
		return nil, err
	}

	return &Rabbit{connection: con, channel: channelRabbitMQ, consume: messages, queue: q}, nil
}

func (r *Rabbit) Stop() {
	r.connection.Close()
	r.channel.Close()
}

func (r *Rabbit) Read(onMsg func([]byte) error) {
	for message := range r.consume {
		message.Ack(true)
		if err := onMsg(message.Body); err != nil {
			log.Printf(" > Received message: %s\n", message.Body)
		}
	}
}

func (r *Rabbit) Publish(msg []byte) error {
	err := r.channel.Publish(
		"",           // exchange
		r.queue.Name, // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        msg,
		})

	return err
}
