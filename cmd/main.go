package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"

	appservice "go.orx.me/apps/hyper-sync/internal/app"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/http"
)

// Global API server instance
var apiServer *appservice.ApiServer

func NewApp() *app.App {
	appCore := core.New(&app.Config{
		Config:   conf.Conf,
		Service:  "hyper-sync",
		Router:   setupRouter,
		InitFunc: []func() error{initServices, initSyncJob},
	})
	return appCore
}

// setupRouter configures the HTTP router with API server integration
func setupRouter(r *gin.Engine) {
	// Initialize API server if not already done
	if apiServer == nil {
		var err error
		apiServer, err = appservice.NewApiServer()
		if err != nil {
			log.Printf("Failed to create API server: %v", err)
			return
		}
	}

	// Setup HTTP routes
	http.Router(r)
}

// initServices initializes all application services
func initServices() error {
	log.Println("Initializing application services...")

	if apiServer == nil {
		var err error
		apiServer, err = appservice.NewApiServer()
		if err != nil {
			log.Printf("Failed to create API server: %v", err)
			return err
		}
	}

	// Get MongoDB client from the core framework
	// This is a placeholder - you'll need to adapt this to your framework
	mongoClient := getMongoDBClient()
	if mongoClient == nil {
		log.Println("MongoDB client not available, skipping service initialization")
		return nil
	}

	// Initialize the API server with MongoDB client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := apiServer.Initialize(ctx, mongoClient)
	if err != nil {
		log.Printf("Failed to initialize API server: %v", err)
		return err
	}

	log.Println("Application services initialized successfully")
	return nil
}

// initSyncJob initializes and optionally starts the sync job
func initSyncJob() error {
	log.Println("Initializing sync job...")

	if apiServer == nil || !apiServer.IsInitialized() {
		log.Println("API server not initialized, skipping sync job initialization")
		return nil
	}

	syncService := apiServer.GetSyncService()
	if syncService == nil {
		log.Println("Sync service not available, skipping sync job initialization")
		return nil
	}

	// Start the sync job
	ctx := context.Background()
	err := apiServer.StartSyncJob(ctx)
	if err != nil {
		log.Printf("Failed to start sync job: %v", err)
		return err
	}

	log.Println("Sync job initialized successfully")
	return nil
}

// getMongoDBClient gets the MongoDB client from the framework
// This is a placeholder - replace with your actual implementation
func getMongoDBClient() *mongo.Client {
	// In a real implementation, you would:
	// 1. Get connection string from configuration
	// 2. Create and return a MongoDB client
	// 3. Handle connection errors appropriately

	// For now, return nil to indicate MongoDB is not available
	// This allows the application to start without database connectivity
	return nil
}

// gracefulShutdown handles application shutdown
func gracefulShutdown() {
	log.Println("Starting graceful shutdown...")

	if apiServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := apiServer.Shutdown(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}

	log.Println("Graceful shutdown completed")
}

func main() {
	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the application in a goroutine
	go func() {
		app := NewApp()
		app.Run()
	}()

	// Wait for shutdown signal
	sig := <-signalChan
	log.Printf("Received signal: %v", sig)

	// Perform graceful shutdown
	gracefulShutdown()
}
