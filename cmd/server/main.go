package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

const RABBIT_URL string = "amqp://guest:guest@localhost:5672/"

func main() {
	fmt.Println("Starting u Peril server...")
	mb, e := amqp.Dial(RABBIT_URL)
	if e != nil {
		log.Fatal(e)
	}
	defer mb.Close()

	fmt.Println("Rabbit MQ Connected")

	ic := make(chan os.Signal, 1)
	signal.Notify(ic, os.Interrupt)

	ch, e := mb.Channel()
	if e != nil {
		log.Fatal(e)
	}

	defer ch.Close()

	pubsub.SubscribeGob(mb, routing.ExchangePerilTopic, routing.GameLogSlug, routing.GameLogSlug+".*", pubsub.DURABLE, func(gl routing.GameLog) pubsub.Acktype {
		defer fmt.Print("> ")
		gamelogic.WriteLog(gl)
		return pubsub.ACK
	})

	gamelogic.PrintServerHelp()

	for {
		q := false
		c := gamelogic.GetInput()

		if len(c) == 0 {
			continue
		}
		switch c[0] {
		default:
			continue
		case "resume":
			fmt.Println("Sending Resume Message")

			if err := pubsub.PublishJSON[routing.PlayingState](ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: false}); err != nil {
				log.Fatal(err)
			}
		case "pause":
			fmt.Println("Sending Pause Message")
			if err := pubsub.PublishJSON[routing.PlayingState](ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: true}); err != nil {
				log.Fatal(err)
			}
		case "quit":
			q = true
			break
		}

		if q {
			break
		}

	}

	<-ic
	fmt.Println("the program is shutting down")

}
