// Package main demonstrates basic usage of the PandaFuzz Go client SDK
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pandafuzz "github.com/ethpandaops/pandafuzz/clients/go"
	"github.com/ethpandaops/pandafuzz/clients/go/generated"
)

func main() {
	// Create a new simple client
	client, err := pandafuzz.NewSimpleClient(
		"http://localhost:8080",
		pandafuzz.WithSimpleAPIKey("your-api-key"),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check system health
	fmt.Println("=== Health Check ===")
	healthResp, err := client.Health(ctx)
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("Health check status: %d\n", healthResp.StatusCode)
		healthResp.Body.Close()
	}

	// List bots
	fmt.Println("\n=== List Bots ===")
	botsResp, err := client.ListBots(ctx, nil)
	if err != nil {
		log.Printf("Failed to list bots: %v", err)
	} else {
		fmt.Printf("List bots status: %d\n", botsResp.StatusCode)
		botsResp.Body.Close()
	}

	// Create a new bot (this will likely fail without proper authentication)
	fmt.Println("\n=== Create Bot ===")
	createReq := generated.BotCreateRequest{
		ApiEndpoint:  "http://localhost:9090",
		Hostname:     "example-bot",
		Capabilities: []generated.BotCreateRequestCapabilities{},
	}

	createResp, err := client.CreateBot(ctx, createReq)
	if err != nil {
		log.Printf("Failed to create bot: %v", err)
	} else {
		fmt.Printf("Create bot status: %d\n", createResp.StatusCode)
		createResp.Body.Close()
	}

	// List campaigns
	fmt.Println("\n=== List Campaigns ===")
	campaignsResp, err := client.ListCampaigns(ctx, nil)
	if err != nil {
		log.Printf("Failed to list campaigns: %v", err)
	} else {
		fmt.Printf("List campaigns status: %d\n", campaignsResp.StatusCode)
		campaignsResp.Body.Close()
	}

	fmt.Println("\nExample completed!")
}
