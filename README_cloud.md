# Shelly Cloud API Device Status Logger for InfluxDB

This tool polls Shelly smart devices via the **official Shelly Cloud API** and logs their on/off status (and other metrics) to InfluxDB at configurable intervals.

## Important: Remote Access via Shelly Cloud

This version uses the **Shelly Cloud API** to access devices remotely, meaning:
- ✅ Works when devices are in a different network/location
- ✅ No need for local network access or port forwarding
- ✅ Access devices anywhere they have internet connection
- ⚠️ Requires devices to be connected to Shelly Cloud
- ⚠️ Rate limited to 1 request per second by Shelly Cloud

## Features

- ✅ Remote device monitoring via Shelly Cloud API
- ✅ Supports Gen1 and Gen2+ Shelly devices
- ✅ Logs on/off status, power consumption, voltage, current, energy, temperature
- ✅ Multi-channel device support (Plus 2PM, Pro 2PM, etc.)
- ✅ Configurable polling interval (minimum 5 minutes recommended)
- ✅ Handles device offline scenarios gracefully
- ✅ Docker support for easy deployment
- ✅ Configuration via YAML file or environment variables

## Prerequisites

### 1. Get Your Shelly Cloud Credentials

You need two pieces of information from the Shelly mobile app:

1. **Open Shelly App** on your phone
2. Go to **User Settings** (profile icon)
3. Tap on **"Authorization cloud key"**
4. You'll see:
   - **Server URI**: e.g., `shelly-103-eu.shelly.cloud`
   - **Auth Key**: Long alphanumeric string

⚠️ **Security Note**: Keep your auth key private! Anyone with this key can control your devices.

### 2. Get Device IDs

For each device you want to monitor:

1. Open device in Shelly App
2. Go to **Settings** → **Device Information**
3. Copy the **Device ID** (hexadecimal format, e.g., `a8032abe41fc`)

## Quick Start

### 1. Configure Your Devices

Edit `config_cloud.yaml`:

```yaml
shelly_cloud:
  server_uri: "shelly-103-eu.shelly.cloud"  # Your server URI
  auth_key: "your-auth-key-here"  # Your authorization key

influxdb:
  url: "http://localhost:8086"
  token: "your-influxdb-token"
  org: "your-org"
  bucket: "shelly_status"

poll_interval: 5  # minutes (minimum 5 recommended)

devices:
  - name: "pump"
    id: "a8032abe41fc"  # Device ID from Shelly App
    type: "plus1pm"
    channel: 0
```

### 2. Run with Docker (Recommended)

```bash
# Build the image
docker build -t shelly-cloud-logger .

# Run with config file
docker run -d \
  --name shelly-cloud-logger \
  --restart unless-stopped \
  -v $(pwd)/config_cloud.yaml:/app/config.yaml:ro \
  shelly-cloud-logger
```

### 3. Or Run with Docker Compose

Create a `.env` file:
```
SHELLY_SERVER_URI=shelly-103-eu.shelly.cloud
SHELLY_AUTH_KEY=your-auth-key-here
INFLUXDB_TOKEN=your-token-here
INFLUXDB_ORG=your-org
```

Then:
```bash
docker-compose up -d
```

### 4. Or Run Directly with Python

```bash
# Install dependencies
pip install -r requirements.txt

# Run the script
python shelly_cloud_logger.py
```

## Configuration Details

### Device Configuration

```yaml
devices:
  - name: "friendly_name"      # Your name for the device
    id: "a8032abe41fc"          # Device ID (hex) from Shelly App
    type: "plus1pm"             # Device model (for reference)
    channel: 0                  # Channel number (0 for single-channel)
```

### Multi-Channel Devices

For devices with multiple channels (Plus 2PM, Pro 2PM, Pro 4PM):

```yaml
devices:
  # First channel
  - name: "light_1"
    id: "b48a0a1cd978"
    type: "plus2pm"
    channel: 0
    
  # Second channel  
  - name: "light_2"
    id: "b48a0a1cd978"  # Same device ID
    type: "plus2pm"
    channel: 1
```

### Supported Device Types

The script works with all Shelly devices accessible via Cloud API:
- **Gen2+**: Plus 1PM, Plus 2PM, Pro 1PM, Pro 2PM, Pro 4PM, Mini 1PM, Plus Plug
- **Gen1**: Plug S, 1PM, 2PM, 25, etc.
- **Covers/Rollers**: Plus 2PM in roller mode, etc.
- **Lights**: Plus RGBW, Duo, etc.

## Data Logged to InfluxDB

### Measurement: `shelly_status`

**Tags:**
- `device`: Device name (from config)
- `device_id`: Shelly device ID
- `type`: Device type (from config)

**Fields:**
- `online` (boolean): Whether device is online in Shelly Cloud
- `cloud_accessible` (boolean): Whether cloud API call succeeded
- `output` (boolean): On/off status
- `output_int` (integer): 1 for ON, 0 for OFF (useful for Grafana)
- `power` (float): Current power consumption in Watts
- `energy` (float): Total energy consumed in Wh
- `voltage` (float): Voltage in V (if available)
- `current` (float): Current in A (if available)
- `temperature` (float): Device temperature in °C (if available)

## Grafana Dashboard Example

### Device Status (On/Off)
```flux
from(bucket: "shelly_status")
  |> range(start: -24h)
  |> filter(fn: (r) => r._measurement == "shelly_status")
  |> filter(fn: (r) => r._field == "output_int")
  |> aggregateWindow(every: 5m, fn: last)
```

### Power Consumption Over Time
```flux
from(bucket: "shelly_status")
  |> range(start: -24h)
  |> filter(fn: (r) => r._measurement == "shelly_status")
  |> filter(fn: (r) => r._field == "power")
  |> aggregateWindow(every: 5m, fn: mean)
```

### Device Online Status
```flux
from(bucket: "shelly_status")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "shelly_status")
  |> filter(fn: (r) => r._field == "online")
  |> last()
```

## Rate Limits

The Shelly Cloud API is limited to **1 request per second**. The script automatically:
- Adds 1.1 second delay between device polls
- For 10 devices, one full poll takes ~11 seconds
- Recommended minimum poll interval: **5 minutes**

## Environment Variables

Can override config file settings:

- `CONFIG_FILE`: Path to config file (default: `config.yaml`)
- `SHELLY_SERVER_URI`: Shelly Cloud server URI
- `SHELLY_AUTH_KEY`: Shelly Cloud authorization key
- `INFLUXDB_URL`: InfluxDB server URL
- `INFLUXDB_TOKEN`: InfluxDB authentication token
- `INFLUXDB_ORG`: InfluxDB organization
- `INFLUXDB_BUCKET`: InfluxDB bucket name
- `POLL_INTERVAL`: Polling interval in minutes

## Troubleshooting

### Device shows as offline but is actually online
- Check device is connected to Shelly Cloud (not just local WiFi)
- Verify device shows as online in Shelly mobile app
- Some battery devices may show offline intermittently

### "Invalid auth_key" error
- Verify auth key is copied correctly from Shelly App
- Auth key changes when you change your Shelly account password
- Make sure there are no extra spaces or line breaks

### "Wrong server_uri" error
- Verify server_uri from Shelly App > User Settings > Authorization cloud key
- Server URI is region-specific (e.g., EU, US, Asia)
- Server may change if Shelly migrates your account

### No data in InfluxDB
- Check InfluxDB token has write permissions
- Verify bucket exists
- Check logs: `docker logs shelly-cloud-logger`

### Rate limit errors
- Reduce number of devices or increase poll interval
- Default 1.1s delay between devices should be sufficient
- For many devices, use 10+ minute poll interval

## Differences from Local API Version

| Feature | Cloud API | Local API |
|---------|-----------|-----------|
| **Network Access** | Remote (internet) | Local only |
| **Requires Cloud** | Yes | No |
| **Rate Limits** | 1 req/sec | None |
| **Latency** | Higher | Lower |
| **Privacy** | Data via Shelly Cloud | Fully local |
| **Setup** | Easier (just auth key) | Requires local IPs |

## Security Considerations

- Store your auth key securely (use environment variables or secrets)
- Don't commit auth key to version control
- Auth key provides full control over your devices
- Consider using read-only monitoring if available (currently not supported by API)

## License

MIT
