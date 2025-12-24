package apidocs

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

//go:embed swagger-ui/*
var swaggerUIFiles embed.FS

//go:embed templates/*
var templateFiles embed.FS

// Config represents the API documentation server configuration
type Config struct {
	Port           int    `json:"port" yaml:"port"`
	OpenAPIPath    string `json:"openapi_path" yaml:"openapi_path"`
	BasePath       string `json:"base_path" yaml:"base_path"`
	Title          string `json:"title" yaml:"title"`
	EnableTryItOut bool   `json:"enable_try_it_out" yaml:"enable_try_it_out"`
	APIKey         string `json:"api_key" yaml:"api_key"`
}

// Server serves the interactive API documentation
type Server struct {
	config  *Config
	logger  *logrus.Logger
	openAPI map[string]interface{}
}

// NewServer creates a new API documentation server
func NewServer(config *Config, logger *logrus.Logger) (*Server, error) {
	if config == nil {
		config = &Config{
			Port:           8081,
			OpenAPIPath:    "./api_v3/openapi.yaml",
			BasePath:       "/api/docs",
			Title:          "PandaFuzz API Documentation",
			EnableTryItOut: true,
		}
	}

	if logger == nil {
		logger = logrus.New()
	}

	// Load OpenAPI specification
	openAPIData, err := os.ReadFile(config.OpenAPIPath)
	if err != nil {
		// Try relative to master package
		altPath := filepath.Join("pkg", "master", "api_v3", "openapi.yaml")
		openAPIData, err = os.ReadFile(altPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
		}
	}

	var openAPI map[string]interface{}
	if err := yaml.Unmarshal(openAPIData, &openAPI); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	return &Server{
		config:  config,
		logger:  logger,
		openAPI: openAPI,
	}, nil
}

// Start starts the API documentation server
func (s *Server) Start() error {
	router := chi.NewRouter()

	// Serve OpenAPI spec
	router.Get(s.config.BasePath+"/openapi.json", s.handleOpenAPISpec)

	// Serve Swagger UI
	router.Get(s.config.BasePath, s.handleSwaggerUI)
	router.Get(s.config.BasePath+"/", s.handleSwaggerUI)

	// Serve static Swagger UI files
	router.Handle(s.config.BasePath+"/static/*",
		http.StripPrefix(s.config.BasePath+"/static/",
			http.FileServer(http.FS(swaggerUIFiles))))

	// API examples and code generation endpoints
	router.Get(s.config.BasePath+"/examples/{language}", s.handleCodeExamples)
	router.Post(s.config.BasePath+"/test", s.handleAPITest)

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.logger.Infof("Starting API documentation server on http://localhost%s%s", addr, s.config.BasePath)

	return http.ListenAndServe(addr, router)
}

// handleOpenAPISpec serves the OpenAPI specification in JSON format
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(s.openAPI); err != nil {
		s.logger.Errorf("Failed to encode OpenAPI spec: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleSwaggerUI serves the Swagger UI interface
func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	tmplData, err := templateFiles.ReadFile("templates/index.html")
	if err != nil {
		s.logger.Errorf("Failed to read template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New("swagger").Parse(string(tmplData))
	if err != nil {
		s.logger.Errorf("Failed to parse template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Title          string
		OpenAPIURL     string
		EnableTryItOut bool
		APIKey         string
	}{
		Title:          s.config.Title,
		OpenAPIURL:     s.config.BasePath + "/openapi.json",
		EnableTryItOut: s.config.EnableTryItOut,
		APIKey:         s.config.APIKey,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Errorf("Failed to execute template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleCodeExamples generates code examples for different languages
func (s *Server) handleCodeExamples(w http.ResponseWriter, r *http.Request) {
	language := chi.URLParam(r, "language")

	examples := s.generateCodeExamples(language)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"language": language,
		"examples": examples,
	})
}

// generateCodeExamples generates code examples for a specific language
func (s *Server) generateCodeExamples(language string) []CodeExample {
	examples := []CodeExample{}

	switch language {
	case "python":
		examples = append(examples, CodeExample{
			Title:       "Create a Job",
			Description: "Example of creating a new fuzzing job",
			Code: `import requests

api_url = "http://localhost:8080/api/v1"
api_key = "your-api-key"

# Create a new fuzzing job
job_data = {
    "name": "Test Fuzzing Job",
    "target": "/path/to/target",
    "fuzzer": "afl++",
    "duration": 3600,
    "config": {
        "threads": 4,
        "memory_limit": 1024
    }
}

response = requests.post(
    f"{api_url}/jobs",
    json=job_data,
    headers={"X-API-Key": api_key}
)

if response.status_code == 201:
    job = response.json()
    print(f"Job created: {job['id']}")
`,
		})

	case "go":
		examples = append(examples, CodeExample{
			Title:       "List Jobs",
			Description: "Example of listing fuzzing jobs",
			Code: `package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

func main() {
    apiURL := "http://localhost:8080/api/v1"
    apiKey := "your-api-key"

    // List jobs with filtering
    req, _ := http.NewRequest("GET", apiURL+"/jobs?status=running", nil)
    req.Header.Set("X-API-Key", apiKey)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var jobs []map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&jobs)
    
    fmt.Printf("Found %d running jobs\n", len(jobs))
}`,
		})

	case "curl":
		examples = append(examples, CodeExample{
			Title:       "Get Campaign Stats",
			Description: "Example of retrieving campaign statistics",
			Code: `# Get campaign statistics
curl -X GET "http://localhost:8080/api/v1/campaigns/{campaign_id}/stats" \
  -H "X-API-Key: your-api-key" \
  -H "Accept: application/json"

# Get coverage trend for last 24 hours
curl -X GET "http://localhost:8080/api/v1/analytics/coverage-trend?campaign_id={campaign_id}&period=24h" \
  -H "X-API-Key: your-api-key" \
  -H "Accept: application/json"`,
		})
	}

	return examples
}

// handleAPITest provides a simple API testing interface
func (s *Server) handleAPITest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var testRequest APITestRequest
	if err := json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// In a real implementation, this would make the actual API call
	// For now, return a mock response
	response := APITestResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: map[string]interface{}{
			"message": "This is a test response",
			"request": testRequest,
		},
		Duration: "125ms",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CodeExample represents a code example for API usage
type CodeExample struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Code        string `json:"code"`
}

// APITestRequest represents a test API request
type APITestRequest struct {
	Method  string                 `json:"method"`
	Path    string                 `json:"path"`
	Headers map[string]string      `json:"headers"`
	Body    map[string]interface{} `json:"body"`
}

// APITestResponse represents a test API response
type APITestResponse struct {
	StatusCode int                    `json:"status_code"`
	Headers    map[string]string      `json:"headers"`
	Body       map[string]interface{} `json:"body"`
	Duration   string                 `json:"duration"`
}
