package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	ShellyCloud ShellyCloudConfig `yaml:"shelly_cloud"`
	InfluxDB    InfluxDBConfig    `yaml:"influxdb"`
	PollInterval int              `yaml:"poll_interval"`
	Devices     []DeviceConfig    `yaml:"devices"`
}

type ShellyCloudConfig struct {
	ServerURI string `yaml:"server_uri"`
	AuthKey   string `yaml:"auth_key"`
}

type InfluxDBConfig struct {
	URL    string `yaml:"url"`
	Token  string `yaml:"token"`
	Org    string `yaml:"org"`
	Bucket string `yaml:"bucket"`
}

type DeviceConfig struct {
	Name    string `yaml:"name"`
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Channel int    `yaml:"channel"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig() (*Config, error) {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config.yaml"
	}

	config := &Config{
		PollInterval: 5, // Default
	}

	// Try to load from YAML file
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("Loading configuration from %s\n", configFile)
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	} else {
		fmt.Println("Config file not found, using defaults and environment variables")
	}

	// Override with environment variables
	if val := os.Getenv("SHELLY_SERVER_URI"); val != "" {
		config.ShellyCloud.ServerURI = val
	}
	if val := os.Getenv("SHELLY_AUTH_KEY"); val != "" {
		config.ShellyCloud.AuthKey = val
	}

	if val := os.Getenv("INFLUXDB_URL"); val != "" {
		config.InfluxDB.URL = val
	}
	if val := os.Getenv("INFLUXDB_TOKEN"); val != "" {
		config.InfluxDB.Token = val
	}
	if val := os.Getenv("INFLUXDB_ORG"); val != "" {
		config.InfluxDB.Org = val
	}
	if val := os.Getenv("INFLUXDB_BUCKET"); val != "" {
		config.InfluxDB.Bucket = val
	}

	if val := os.Getenv("POLL_INTERVAL"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			config.PollInterval = i
		}
	}

	// Defaults for InfluxDB if missing
	if config.InfluxDB.URL == "" {
		config.InfluxDB.URL = "http://localhost:8086"
	}
	if config.InfluxDB.Bucket == "" {
		config.InfluxDB.Bucket = "shelly_status"
	}

	return config, nil
}
