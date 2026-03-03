package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril client...")
	mbc, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatal(err)
	}
	defer mbc.Close()
	uname, err := gamelogic.ClientWelcome()

	if err != nil {
		log.Fatal(err)
	}

	gs := gamelogic.NewGameState(uname)

	ch, e := mbc.Channel()
	if e != nil {
		log.Fatal(e)
	}
	if err := pubsub.SubscribeJSON(mbc, routing.ExchangePerilDirect, routing.PauseKey+"."+uname, routing.PauseKey, pubsub.TRANSIENT, handlerPause(gs)); err != nil {
		log.Fatal(err)
	}

	if err := pubsub.SubscribeJSON(mbc, routing.ExchangePerilTopic, "army_moves."+uname, "army_moves.*", pubsub.TRANSIENT, handlerMove(ch, gs)); err != nil {
		log.Fatal(err)
	}

	if err := pubsub.SubscribeJSON(mbc, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix, routing.WarRecognitionsPrefix+".*", pubsub.DURABLE, handlerWar(ch, gs)); err != nil {
		log.Fatal(err)
	}
	for {
		q := false
		c := gamelogic.GetInput()
		if len(c) == 0 {
			continue
		}
		switch c[0] {
		case "spawn":
			gs.CommandSpawn(c)
		case "move":
			mv, e := gs.CommandMove(c)
			if e != nil {
				log.Fatal(e)
			}
			if err := pubsub.PublishJSON(ch, routing.ExchangePerilTopic, "army_moves."+uname, mv); err != nil {
				log.Fatal(err)
			}
			log.Println(err)
			fmt.Println("move was published")
		case "status":
			gs.CommandStatus()
		case "help":
			gamelogic.PrintClientHelp()
		case "spam":
			log.Println("spamming ", c)
			n, err := strconv.Atoi(c[1])
			if err != nil {
				log.Fatal(err)
			}
			for range n {
				ml := gamelogic.GetMaliciousLog()
				PublishGameLog(ch, uname, ml)
			}
		case "quit":
			gamelogic.PrintQuit()
			q = true
		default:
			fmt.Println("Error: Invalid Input")
			continue
		}
		if q {
			break
		}
	}

	ich := make(chan os.Signal, 1)
	signal.Notify(ich, os.Interrupt)
	<-ich

}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.Acktype {
	return func(ps routing.PlayingState) pubsub.Acktype {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.ACK
	}
}

func handlerMove(ch *amqp.Channel, gs *gamelogic.GameState) func(gamelogic.ArmyMove) pubsub.Acktype {

	return func(mv gamelogic.ArmyMove) pubsub.Acktype {
		defer fmt.Print("> ")
		mo := gs.HandleMove(mv)
		switch mo {
		case gamelogic.MoveOutComeSafe:
			return pubsub.ACK
		case gamelogic.MoveOutcomeMakeWar:
			if err := pubsub.PublishJSON(ch, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix+"."+mv.Player.Username, gamelogic.RecognitionOfWar{
				Attacker: mv.Player,
				Defender: gs.GetPlayerSnap(),
			}); err != nil {
				return pubsub.NACK
			}
			return pubsub.ACK
		}
		return pubsub.ACK
	}
}

func handlerWar(ch *amqp.Channel, gs *gamelogic.GameState) func(gamelogic.RecognitionOfWar) pubsub.Acktype {

	return func(mv gamelogic.RecognitionOfWar) pubsub.Acktype {
		defer fmt.Print("> ")
		wo, w, l := gs.HandleWar(mv)
		var logMsg string
		switch wo {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NACK
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NACK_DISCARD
		case gamelogic.WarOutcomeOpponentWon:
			logMsg = fmt.Sprint(w, " won a war against ", l)
			if err := PublishGameLog(ch, gs.GetUsername(), logMsg); err != nil {
				return pubsub.NACK
			}
			return pubsub.ACK
		case gamelogic.WarOutcomeYouWon:
			logMsg = fmt.Sprint(w, " won a war against ", l)
			if err := PublishGameLog(ch, gs.GetUsername(), logMsg); err != nil {
				return pubsub.NACK
			}
			return pubsub.ACK
		case gamelogic.WarOutcomeDraw:
			logMsg = fmt.Sprint("A war between ", w, " and ", l, " resulted in a draw")
			if err := PublishGameLog(ch, gs.GetUsername(), logMsg); err != nil {
				return pubsub.NACK
			}
			return pubsub.ACK
		default:
			return pubsub.NACK_DISCARD
		}
	}
}

func PublishGameLog(ch *amqp.Channel, uname, msg string) error {
	e := pubsub.PublishGob(ch, routing.ExchangePerilTopic, routing.GameLogSlug+"."+uname, routing.GameLog{
		Username:    uname,
		CurrentTime: time.Now(),
		Message:     msg,
	})
	log.Println(e)
	return e
}
