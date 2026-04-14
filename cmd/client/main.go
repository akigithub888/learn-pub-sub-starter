package main

import (
	"fmt"

	"github.com/akigithub888/learn-file-storage-s3-golang-starter/internal/gamelogic"
	"github.com/akigithub888/learn-file-storage-s3-golang-starter/internal/pubsub"
	"github.com/akigithub888/learn-file-storage-s3-golang-starter/internal/routing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	cString := "amqp://guest:guest@localhost:5672/"

	connection, err := amqp.Dial(cString)
	if err != nil {
		panic(err)
	}

	channel, err := connection.Channel()
	if err != nil {
		panic(err)
	}
	defer channel.Close()

	//err = pubsub.PublishJSON(channel, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
	//IsPaused: true,
	//})

	defer connection.Close()

	fmt.Println("Starting Peril server...")
	fmt.Println("Connected to RabbitMQ at:", cString)

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		fmt.Println(err)
		return
	}
	queueName := routing.PauseKey + "." + username

	newState := gamelogic.NewGameState(username)

	err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilDirect,
		queueName,
		routing.PauseKey,
		pubsub.QueueTypeTransient,
		handlerPause(newState),
	)
	if err != nil {
		fmt.Printf("Error subscribing to pause messages: %s\n", err)
		return
	}

	err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+username,
		routing.ArmyMovesPrefix+".*",
		pubsub.QueueTypeTransient,
		handlerMove(newState, channel),
	)

	if err != nil {
		fmt.Printf("Error subscribing to army move messages: %s\n", err)
		return
	}

	err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilTopic,
		"war",
		routing.WarRecognitionsPrefix+".*",
		pubsub.QueueTypeDurable,
		handlerAllWarMessages(newState),
	)

	if err != nil {
		fmt.Printf("Error subscribing to war recognition messages: %s\n", err)
		return
	}

	for {
		input := gamelogic.GetInput()

		if len(input) == 0 {
			continue
		}

		if input[0] == "spawn" {
			if err := newState.CommandSpawn(input); err != nil {
				fmt.Println(err)
			}
		} else if input[0] == "move" {
			moveResult, err := newState.CommandMove(input)
			if err != nil {
				fmt.Println(err)
			} else {
				err = pubsub.PublishJSON(
					channel,
					routing.ExchangePerilTopic,
					routing.ArmyMovesPrefix+"."+username,
					moveResult,
				)
				if err != nil {
					fmt.Printf("Error publishing move result: %s\n", err)
				}
				// publish move result to other players
				fmt.Printf("Published move result for %s\n", username)
			}
		} else if input[0] == "status" {
			newState.CommandStatus()
		} else if input[0] == "help" {
			gamelogic.PrintClientHelp()
		} else if input[0] == "spam" {
			fmt.Println("Spamming not allowed yet!")
		} else if input[0] == "quit" {
			gamelogic.PrintQuit()
			return
		} else {
			fmt.Printf("Unknown command: %s\n", input[0])
			continue
		}
	}

	// wait for ctrl+c
	//signalChan := make(chan os.Signal, 1)
	//signal.Notify(signalChan, os.Interrupt)
	//recievedSignal := <-signalChan
	//fmt.Printf("Recieved signal: %s. Shutting down...\n", recievedSignal)

}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckAction {
	return func(ps routing.PlayingState) pubsub.AckAction {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.ArmyMove) pubsub.AckAction {
	return func(am gamelogic.ArmyMove) pubsub.AckAction {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(am)

		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack

		case gamelogic.MoveOutcomeMakeWar:
			err := pubsub.PublishJSON(
				ch,
				routing.ExchangePerilTopic,
				routing.WarRecognitionsPrefix+"."+gs.GetUsername(),
				gamelogic.RecognitionOfWar{
					Attacker: am.Player,
					Defender: gs.GetPlayerSnap(),
				},
			)
			if err != nil {
				fmt.Printf("Error publishing war recognition: %s\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.Ack
		default:
			return pubsub.NackDiscard
		}
	}
}

func handlerAllWarMessages(gs *gamelogic.GameState) func(recognition gamelogic.RecognitionOfWar) pubsub.AckAction {
	return func(recognition gamelogic.RecognitionOfWar) pubsub.AckAction {
		defer fmt.Print("> ")
		outcome, _, _ := gs.HandleWar(recognition)
		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeOpponentWon:
			return pubsub.Ack
		case gamelogic.WarOutcomeYouWon:
			return pubsub.Ack
		case gamelogic.WarOutcomeDraw:
			return pubsub.Ack
		default:
			fmt.Printf("Error handling war message: unrecognized outcome %v\n", outcome)
			return pubsub.NackDiscard
		}
	}
}
