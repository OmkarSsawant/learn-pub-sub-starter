package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"

	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type SimpleQueueType string

const (
	DURABLE   SimpleQueueType = "durable"
	TRANSIENT SimpleQueueType = "transient"
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	b, e := json.Marshal(val)
	if e != nil {
		return e
	}
	return ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        b,
	})
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	var buf bytes.Buffer
	en := gob.NewEncoder(&buf)
	if e := en.Encode(val); e != nil {
		return e
	}
	return ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/gob",
		Body:        buf.Bytes(),
	})
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // SimpleQueueType is an "enum" type I made to represent "durable" or "transient"
) (*amqp.Channel, amqp.Queue, error) {
	var que amqp.Queue
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	tbl := amqp.Table{
		"x-dead-letter-exchange": "peril_dlx",
	}

	if queueType == TRANSIENT {
		q, e := ch.QueueDeclare(queueName, false, true, true, false, tbl)
		if e != nil {
			log.Fatal(e)
		}
		que = q
	} else {
		q, e := ch.QueueDeclare(queueName, true, false, false, false, tbl)
		if e != nil {
			log.Fatal(e)
		}
		que = q
	}

	if err := ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
		log.Fatal(err)
	}

	return ch, que, err

}

type Acktype string

const (
	ACK                  = "Ack"
	NACK                 = "NackRequeue"
	NACK_DISCARD Acktype = "NackDiscard"
)

func Subscribe[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // an enum to represent "durable" or "transient"
	handler func(T) Acktype,
	unmarshal func([]byte) (T, error),
) error {
	DeclareAndBind(conn, exchange, queueName, key, queueType)
	ch, err := conn.Channel()
	if err != nil {
		return err
	}

	ch.Qos(10, 0, false)
	dil, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for msg := range dil {
			if m, e := unmarshal(msg.Body); e == nil {
				switch handler(m) {
				case ACK:
					log.Println("Ack")
					msg.Ack(false)
				case NACK:
					log.Println("Nack->Requeued")
					msg.Nack(false, true)
				case NACK_DISCARD:
					log.Println("Nack->Discarded")
					msg.Nack(false, false)
				}

			}
		}
	}()
	return nil
}

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // an enum to represent "durable" or "transient"
	handler func(T) Acktype,
) error {
	return Subscribe(conn, exchange, queueName, key, queueType, handler, func(b []byte) (T, error) {
		var m T
		e := json.Unmarshal(b, &m)
		return m, e
	})
}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // an enum to represent "durable" or "transient"
	handler func(T) Acktype,
) error {
	return Subscribe(conn, exchange, queueName, key, queueType, handler, func(b []byte) (T, error) {
		var m T
		dec := gob.NewDecoder(bytes.NewBuffer(b))
		e := dec.Decode(&m)
		return m, e
	})
}
