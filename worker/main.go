package main

import (
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"os"
	"temporal-ecommerce/app"
)

func main() {
	// Check environment variables
	stripeKey := os.Getenv("STRIPE_PRIVATE_KEY")
	if stripeKey == "" {
		panic("Must set STRIPE_PRIVATE_KEY environment variable")
	}

	// Create the client object just once per process
	c, err := client.NewClient(client.Options{})
	if err != nil {
		log.Fatalln("unable to create Temporal client", err)
	}
	defer c.Close()
	// This worker hosts both Worker and Activity functions
	w := worker.New(c, "CART_TASK_QUEUE", worker.Options{})

	w.RegisterActivity(app.CreateStripeCharge)
	w.RegisterWorkflow(app.CartWorkflow)
	// Start listening to the Task Queue
	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalln("unable to start Worker", err)
	}
}
