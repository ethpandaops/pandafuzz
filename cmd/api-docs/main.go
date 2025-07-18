package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ethpandaops/pandafuzz/pkg/web/api_docs"
	"github.com/sirupsen/logrus"
)

func main() {
	var (
		port        = flag.Int("port", 8081, "Port to serve API documentation")
		openAPIPath = flag.String("openapi", "", "Path to OpenAPI specification file")
		basePath    = flag.String("base-path", "/api/docs", "Base path for API documentation")
		apiKey      = flag.String("api-key", "", "API key for testing (optional)")
		verbose     = flag.Bool("v", false, "Enable verbose logging")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "PandaFuzz API Documentation Server\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                    # Start with defaults\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -port 8080         # Use custom port\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -api-key test123   # Enable API testing with key\n", os.Args[0])
	}

	flag.Parse()

	// Configure logger
	logger := logrus.New()
	if *verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	// Create configuration
	config := &apidocs.Config{
		Port:           *port,
		BasePath:       *basePath,
		Title:          "PandaFuzz API Documentation",
		EnableTryItOut: true,
		APIKey:         *apiKey,
	}

	if *openAPIPath != "" {
		config.OpenAPIPath = *openAPIPath
	}

	// Create and start server
	server, err := apidocs.NewServer(config, logger)
	if err != nil {
		logger.Fatalf("Failed to create API documentation server: %v", err)
	}

	logger.Infof("Starting API documentation server on port %d", config.Port)
	logger.Infof("Access the documentation at http://localhost:%d%s", config.Port, config.BasePath)

	if err := server.Start(); err != nil {
		logger.Fatalf("Server failed: %v", err)
	}
}
