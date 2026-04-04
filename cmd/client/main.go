package main

import (
	"fmt"
	"os"
	"os/signal"

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

	err = pubsub.PublishJSON(channel, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
		IsPaused: true,
	})

	defer connection.Close()

	fmt.Println("Starting Peril server...")
	fmt.Println("Connected to RabbitMQ at:", cString)

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		fmt.Println(err)
		return
	}
	queueName := routing.PauseKey + "." + username

	_, _, err = pubsub.DeclareAndBind(
		connection,
		routing.ExchangePerilDirect,
		queueName,
		routing.PauseKey,
		pubsub.QueueTypeTransient,
	)
	if err != nil {
		panic(err)
	}

	newState := gamelogic.NewGameState(username)

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
				fmt.Printf("Moved %d unit(s) to %s: ", len(moveResult.Units), moveResult.ToLocation)
				for i, unit := range moveResult.Units {
					if i > 0 {
						fmt.Print(", ")
					}
					fmt.Print(unit.Rank)
				}
				fmt.Println()
			}
		} else if input[0] == "status" {
			newState.CommandStatus()
		} else if input[0] == "help" {
			gamelogic.PrintClientHelp()
		} else if input[0] == "spam" {
			fmt.Println("Spamming not allowed yet!")
		} else if input[0] == "quit" {
			gamelogic.PrintQuit()
			break
		} else {
			fmt.Printf("Unknown command: %s\n", input[0])
			continue
		}
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	recievedSignal := <-signalChan
	fmt.Printf("Recieved signal: %s. Shutting down...\n", recievedSignal)

}
