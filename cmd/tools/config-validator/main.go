package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		configFile      = flag.String("config", "", "Path to configuration file to validate")
		outputJSON      = flag.Bool("json", false, "Output validation result as JSON")
		showDefaults    = flag.Bool("defaults", false, "Show configuration with defaults applied")
		generateExample = flag.String("generate", "", "Generate example config (master-filesystem, master-s3, master-minio, bot)")
	)

	flag.Parse()

	// Handle example generation
	if *generateExample != "" {
		if err := generateExampleConfig(*generateExample); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating example: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Validate config file
	if *configFile == "" {
		fmt.Fprintf(os.Stderr, "Error: -config flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Create config manager
	configMgr := common.NewConfigManager()

	// Load configuration
	config, err := configMgr.LoadConfig(*configFile)
	if err != nil {
		if *outputJSON {
			result := map[string]interface{}{
				"valid": false,
				"error": err.Error(),
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		}
		os.Exit(1)
	}

	// Show configuration with defaults if requested
	if *showDefaults {
		// Apply defaults manually since setDefaults is private
		if config.Master != nil {
			configMgr.SetMasterDefaults(config.Master)
			config.Master.Storage.SetDefaults()
		}
		// Bot defaults would be applied here if needed

		// Output as YAML
		data, err := yaml.Marshal(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	// Output validation result
	if *outputJSON {
		result := map[string]interface{}{
			"valid": true,
			"type":  getConfigType(config),
		}

		// Add configuration details
		if config.Master != nil {
			result["master"] = map[string]interface{}{
				"server_port":   config.Master.Server.Port,
				"database_type": config.Master.Database.Type,
				"storage_type":  config.Master.Storage.Type,
				"monitoring":    config.Master.Monitoring.Enabled,
			}
		}

		if config.Bot != nil {
			result["bot"] = map[string]interface{}{
				"id":           config.Bot.ID,
				"master_url":   config.Bot.MasterURL,
				"capabilities": config.Bot.Capabilities,
			}
		}

		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✓ Configuration is valid\n")
		fmt.Printf("  Type: %s\n", getConfigType(config))

		if config.Master != nil {
			fmt.Printf("\nMaster Configuration:\n")
			fmt.Printf("  Server Port: %d\n", config.Master.Server.Port)
			fmt.Printf("  Database: %s\n", config.Master.Database.Type)
			fmt.Printf("  Storage: %s\n", config.Master.Storage.Type)

			switch config.Master.Storage.Type {
			case "filesystem":
				fmt.Printf("    Base Path: %s\n", config.Master.Storage.Filesystem.BasePath)
			case "s3":
				fmt.Printf("    Region: %s\n", config.Master.Storage.S3.Region)
				fmt.Printf("    Corpus Bucket: %s\n", config.Master.Storage.S3.CorpusBucket)
			case "minio":
				fmt.Printf("    Endpoint: %s\n", config.Master.Storage.MinIO.Endpoint)
				fmt.Printf("    Corpus Bucket: %s\n", config.Master.Storage.MinIO.CorpusBucket)
			}

			fmt.Printf("  Monitoring: %v\n", config.Master.Monitoring.Enabled)
			if config.Master.Monitoring.Enabled {
				fmt.Printf("    Metrics Port: %d\n", config.Master.Monitoring.MetricsPort)
			}
		}

		if config.Bot != nil {
			fmt.Printf("\nBot Configuration:\n")
			fmt.Printf("  ID: %s\n", config.Bot.ID)
			fmt.Printf("  Master URL: %s\n", config.Bot.MasterURL)
			fmt.Printf("  Capabilities: %s\n", strings.Join(config.Bot.Capabilities, ", "))
			fmt.Printf("  Work Directory: %s\n", config.Bot.Fuzzing.WorkDir)
			fmt.Printf("  Max Jobs: %d\n", config.Bot.Fuzzing.MaxJobs)
		}
	}
}

func getConfigType(config *common.Config) string {
	types := []string{}
	if config.Master != nil {
		types = append(types, "master")
	}
	if config.Bot != nil {
		types = append(types, "bot")
	}
	if len(types) == 0 {
		return "empty"
	}
	return strings.Join(types, "+")
}

func generateExampleConfig(configType string) error {
	var config interface{}
	var filename string

	switch configType {
	case "master-filesystem":
		filename = "master-filesystem.example.yaml"
		config = generateMasterFilesystemExample()

	case "master-s3":
		filename = "master-s3.example.yaml"
		config = generateMasterS3Example()

	case "master-minio":
		filename = "master-minio.example.yaml"
		config = generateMasterMinIOExample()

	case "bot":
		filename = "bot.example.yaml"
		config = generateBotExample()

	default:
		return fmt.Errorf("unknown config type: %s (valid: master-filesystem, master-s3, master-minio, bot)", configType)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Generated example configuration: %s\n", filename)
	return nil
}

func generateMasterFilesystemExample() map[string]interface{} {
	return map[string]interface{}{
		"master": map[string]interface{}{
			"server": map[string]interface{}{
				"host": "0.0.0.0",
				"port": 8080,
			},
			"database": map[string]interface{}{
				"type": "sqlite",
				"path": "./data/pandafuzz.db",
			},
			"storage": map[string]interface{}{
				"type": "filesystem",
				"filesystem": map[string]interface{}{
					"base_path": "./storage/corpus",
				},
				"max_file_size":      104857600,
				"enable_dedup":       true,
				"enable_compression": false,
			},
			"timeouts": map[string]interface{}{
				"bot_heartbeat":   "60s",
				"job_execution":   "3600s",
				"master_recovery": "300s",
			},
			"limits": map[string]interface{}{
				"max_concurrent_jobs": 10,
				"max_corpus_size":     1073741824,
			},
			"monitoring": map[string]interface{}{
				"enabled":      true,
				"metrics_port": 9090,
			},
		},
	}
}

func generateMasterS3Example() map[string]interface{} {
	return map[string]interface{}{
		"master": map[string]interface{}{
			"server": map[string]interface{}{
				"host": "0.0.0.0",
				"port": 8080,
			},
			"database": map[string]interface{}{
				"type": "postgres",
				"dsn":  "postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:5432/pandafuzz?sslmode=require",
			},
			"storage": map[string]interface{}{
				"type": "s3",
				"s3": map[string]interface{}{
					"region":            "${AWS_REGION}",
					"access_key_id":     "${AWS_ACCESS_KEY_ID}",
					"secret_access_key": "${AWS_SECRET_ACCESS_KEY}",
					"corpus_bucket":     "pandafuzz-corpus-prod",
					"quarantine_bucket": "pandafuzz-quarantine-prod",
					"backup_bucket":     "pandafuzz-backup-prod",
					"use_ssl":           true,
				},
				"max_file_size":      524288000,
				"enable_dedup":       true,
				"enable_compression": true,
			},
			"timeouts": map[string]interface{}{
				"bot_heartbeat":   "120s",
				"job_execution":   "7200s",
				"master_recovery": "600s",
			},
			"limits": map[string]interface{}{
				"max_concurrent_jobs": 100,
				"max_corpus_size":     107374182400,
			},
			"monitoring": map[string]interface{}{
				"enabled":      true,
				"metrics_port": 9090,
			},
		},
	}
}

func generateMasterMinIOExample() map[string]interface{} {
	return map[string]interface{}{
		"master": map[string]interface{}{
			"server": map[string]interface{}{
				"host": "0.0.0.0",
				"port": 8080,
			},
			"database": map[string]interface{}{
				"type": "sqlite",
				"path": "./data/pandafuzz.db",
			},
			"storage": map[string]interface{}{
				"type": "minio",
				"minio": map[string]interface{}{
					"endpoint":          "localhost:9000",
					"access_key_id":     "pandafuzz",
					"secret_access_key": "pandafuzz123",
					"corpus_bucket":     "corpus",
					"quarantine_bucket": "quarantine",
					"backup_bucket":     "backup",
					"use_ssl":           false,
					"use_path_style":    true,
					"health_check":      true,
				},
				"max_file_size":      104857600,
				"enable_dedup":       true,
				"enable_compression": false,
			},
			"timeouts": map[string]interface{}{
				"bot_heartbeat":   "60s",
				"job_execution":   "3600s",
				"master_recovery": "300s",
			},
			"limits": map[string]interface{}{
				"max_concurrent_jobs": 10,
				"max_corpus_size":     1073741824,
			},
			"monitoring": map[string]interface{}{
				"enabled":      true,
				"metrics_port": 9090,
			},
		},
	}
}

func generateBotExample() map[string]interface{} {
	return map[string]interface{}{
		"bot": map[string]interface{}{
			"id":           "bot-${HOSTNAME}-${TIMESTAMP}",
			"name":         "Fuzzing Bot 1",
			"master_url":   "http://localhost:8080",
			"api_port":     9049,
			"capabilities": []string{"libfuzzer", "afl++", "honggfuzz"},
			"fuzzing": map[string]interface{}{
				"work_dir":           "/tmp/pandafuzz",
				"max_jobs":           1,
				"job_cleanup":        true,
				"corpus_sync":        true,
				"crash_reporting":    true,
				"coverage_reporting": true,
			},
			"timeouts": map[string]interface{}{
				"heartbeat_interval":   "30s",
				"job_execution":        "3600s",
				"master_communication": "30s",
			},
			"resources": map[string]interface{}{
				"max_memory_mb":   2048,
				"max_cpu_percent": 80,
			},
			"logging": map[string]interface{}{
				"level":  "info",
				"format": "json",
			},
		},
	}
}
