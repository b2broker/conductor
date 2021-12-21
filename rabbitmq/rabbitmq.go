package rabbitmq

import (
	"fmt"
	"log"

	"github.com/streadway/amqp"
)

type Rabbit struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	consume    <-chan amqp.Delivery
	Queue      amqp.Queue
}

func NewRabbit(con *amqp.Connection, queueName string, exclusive bool) (*Rabbit, error) {

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
		false,     // durable
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

func (r *Rabbit) Read(onMsg func(amqp.Delivery) error) {
	for {
		for message := range r.consume {
			//message.Ack(false)
			fmt.Println("Get rabbit message")
			if err := onMsg(message); err != nil {
				log.Printf(" > Received message: %s\n", message.Body)
			}
		}
	}

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
