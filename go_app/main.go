package main

import (
	"log"
	"os"
	"os/signal"
	"shelly-cloud-logger/shelly"
	"syscall"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

func main() {
	log.Println("=== Starting Shelly Cloud API Status Logger ===")

	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if len(config.Devices) == 0 {
		log.Fatal("No devices configured! Please add devices to config.yaml")
	}
	if config.ShellyCloud.AuthKey == "" {
		log.Fatal("Shelly Cloud auth_key not configured!")
	}
	if config.ShellyCloud.ServerURI == "" {
		log.Fatal("Shelly Cloud server_uri not configured!")
	}
	if config.InfluxDB.Token == "" {
		log.Fatal("InfluxDB token not configured!")
	}

	log.Printf("Monitoring %d devices", len(config.Devices))
	log.Printf("Shelly Cloud: %s", config.ShellyCloud.ServerURI)
	log.Printf("InfluxDB: %s, Bucket: %s", config.InfluxDB.URL, config.InfluxDB.Bucket)
	log.Printf("Poll interval: %d minutes", config.PollInterval)

	// Initialize InfluxDB client
	influxClient := influxdb2.NewClient(config.InfluxDB.URL, config.InfluxDB.Token)
	defer influxClient.Close()
	writeAPI := influxClient.WriteAPI(config.InfluxDB.Org, config.InfluxDB.Bucket)

    // Handle write errors
    errorsCh := writeAPI.Errors()
    go func() {
        for err := range errorsCh {
            log.Printf("Write error: %s\n", err.Error())
        }
    }()

	shellyClient := shelly.NewClient(config.ShellyCloud.ServerURI, config.ShellyCloud.AuthKey)

	// Function to poll devices
	pollDevices := func() {
		log.Printf("Polling %d devices via Shelly Cloud API...", len(config.Devices))
		for _, device := range config.Devices {
			processDevice(shellyClient, writeAPI, device)
			time.Sleep(1100 * time.Millisecond) // Rate limit
		}
		log.Println("Polling complete")
	}

	// Initial poll
	pollDevices()

	// Schedule polling
	ticker := time.NewTicker(time.Duration(config.PollInterval) * time.Minute)
	defer ticker.Stop()

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			pollDevices()
		case <-stop:
			log.Println("Shutting down...")
			writeAPI.Flush()
			return
		}
	}
}

func processDevice(client *shelly.Client, writeAPI api.WriteAPI, device DeviceConfig) {
	rawStatus, err := client.GetDeviceStatusV2(device.ID)
	if err != nil {
		log.Printf("Failed to get status for %s (%s): %v", device.Name, device.ID, err)

		// Log offline status
		p := influxdb2.NewPointWithMeasurement("shelly_status").
			AddTag("device", device.Name).
			AddTag("device_id", device.ID).
			AddTag("type", device.Type).
			AddField("online", false).
			AddField("cloud_accessible", false).
			SetTime(time.Now())
		writeAPI.WritePoint(p)
		return
	}

	status, err := shelly.ParseDeviceStatus(rawStatus, device.Channel)
	if err != nil {
		log.Printf("Failed to parse status for %s: %v", device.Name, err)
		return
	}

	// Create InfluxDB point
	p := influxdb2.NewPointWithMeasurement("shelly_status").
		AddTag("device", device.Name).
		AddTag("device_id", device.ID).
		AddTag("type", device.Type).
		AddField("online", status.Online).
		AddField("cloud_accessible", true).
		AddField("output", status.Output).
		AddField("output_int", map[bool]int{true: 1, false: 0}[status.Output]).
		AddField("power", status.Power).
		AddField("energy", status.Energy).
		SetTime(time.Now())

	if status.Voltage != nil {
		p.AddField("voltage", *status.Voltage)
	}
	if status.Current != nil {
		p.AddField("current", *status.Current)
	}
	if status.Temperature != nil {
		p.AddField("temperature", *status.Temperature)
	}

	writeAPI.WritePoint(p)

	onlineStr := "offline"
	if status.Online {
		onlineStr = "online"
	}
	outputStr := "OFF"
	if status.Output {
		outputStr = "ON"
	}
	log.Printf("✓ %s (%s): %s (%.1fW)", device.Name, onlineStr, outputStr, status.Power)
}
