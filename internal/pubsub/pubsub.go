package pubsub

import (
	"context"
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %s", err)
	}

	ctx := context.Background()
	err = ch.PublishWithContext(
		ctx,
		exchange,
		key,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        jsonBytes,
		},
	)
	return err
}

type SimpleQueueType string

const (
	QueueTypeDurable   SimpleQueueType = "durable"
	QueueTypeTransient SimpleQueueType = "transient"
)

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // SimpleQueueType is an "enum" type I made to represent "durable" or "transient"
) (*amqp.Channel, amqp.Queue, error) {

	channel, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	durable := queueType == QueueTypeDurable
	autoDelete := queueType == QueueTypeTransient
	exclusive := queueType == QueueTypeTransient

	q, err := channel.QueueDeclare(
		queueName,
		durable,
		autoDelete,
		exclusive,
		false,
		nil,
	)

	if err != nil {
		return nil, amqp.Queue{}, err
	}

	err = channel.QueueBind(
		q.Name,
		key,
		exchange,
		false,
		nil,
	)

	if err != nil {
		return nil, amqp.Queue{}, err
	}

	return channel, q, nil
}
