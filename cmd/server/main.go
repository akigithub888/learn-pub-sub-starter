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
	defer connection.Close()

	_, _, err = pubsub.DeclareAndBind(
		connection,
		routing.ExchangePerilTopic,
		routing.GameLogSlug,
		"game_logs.*",
		pubsub.QueueTypeDurable,
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting Peril server...")
	fmt.Println("Connected to RabbitMQ at:", cString)

	gamelogic.PrintServerHelp()

	for {
		input := gamelogic.GetInput()

		if len(input) == 0 {
			continue
		}

		if input[0] == "pause" {
			fmt.Println("Pausing game...")
			err = pubsub.PublishJSON(channel, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: true,
			})

		} else if input[0] == "resume" {
			fmt.Println("Resuming game...")
			err = pubsub.PublishJSON(channel, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: false,
			})

		} else if input[0] == "quit" {
			fmt.Println("Quitting game...")
			break

		} else {
			fmt.Printf("Unknown command: %s\n", input[0])
		}
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	recievedSignal := <-signalChan
	fmt.Printf("Recieved signal: %s. Shutting down...\n", recievedSignal)

}
