package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AckAction int

const (
	Ack AckAction = iota
	NackRequeue
	NackDiscard
)

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckAction,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		func(body []byte) (T, error) {
			var val T
			err := json.Unmarshal(body, &val)
			return val, err
		},
	)
}

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
		amqp.Table{
			"x-dead-letter-exchange": "peril_dlx",
		},
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

func PublishGob(ch *amqp.Channel, exchange, key string, val any) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		log.Fatalf("Error encoding with gob: %s", err)
	}

	ctx := context.Background()
	err = ch.PublishWithContext(
		ctx,
		exchange,
		key,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/gob",
			Body:        buf.Bytes(),
		},
	)
	return err
}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckAction,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		func(body []byte) (T, error) {
			var val T

			buf := bytes.NewBuffer(body)
			dec := gob.NewDecoder(buf)

			err := dec.Decode(&val)
			return val, err
		},
	)
}

func subscribe[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckAction,
	unmarshaller func([]byte) (T, error),
) error {
	ch, q, err := DeclareAndBind(
		conn,
		exchange,
		queueName,
		key,
		queueType,
	)
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			val, err := unmarshaller(msg.Body)
			if err != nil {
				log.Printf("[NACK_DISCARD] Failed to decode: %v | body=%v", err, msg.Body)
				msg.Nack(false, false)
				continue
			}

			action := handler(val)

			switch action {
			case Ack:
				log.Printf("[ACK] message_id=%s routing_key=%s", msg.MessageId, msg.RoutingKey)
				msg.Ack(false)
			case NackRequeue:
				log.Printf("[NACK_REQUEUE] message_id=%s routing_key=%s", msg.MessageId, msg.RoutingKey)
				msg.Nack(false, true)
			case NackDiscard:
				log.Printf("[NACK_DISCARD] message_id=%s routing_key=%s", msg.MessageId, msg.RoutingKey)
				msg.Nack(false, false)
			}
		}
	}()

	return nil
}
