package shelly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DeviceStatus represents the standardized status of a device
type DeviceStatus struct {
	Online      bool
	Output      bool
	Power       float64
	Energy      float64
	Voltage     *float64
	Current     *float64
	Temperature *float64
}

// Client handles Shelly Cloud API requests
type Client struct {
	ServerURI string
	AuthKey   string
	HTTPClient *http.Client
}

// NewClient creates a new Shelly Cloud API client
func NewClient(serverURI, authKey string) *Client {
	return &Client{
		ServerURI: serverURI,
		AuthKey:   authKey,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetDeviceStatusV2 fetches the raw status from Shelly Cloud API
func (c *Client) GetDeviceStatusV2(deviceID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://%s/v2/devices/api/get", c.ServerURI)

	payload := map[string]interface{}{
		"ids":    []string{deviceID},
		"select": []string{"status"},
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("auth_key", c.AuthKey)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var rawData interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// The response is expected to be a map with data keys, or a list in "data"?
    // Looking at python code: `data = response.json()` then `return data[0]`.
    // So the response body is a JSON array.

    // Let's assert it is a list
    if dataList, ok := rawData.([]interface{}); ok {
        if len(dataList) > 0 {
            if deviceData, ok := dataList[0].(map[string]interface{}); ok {
                return deviceData, nil
            }
            return nil, fmt.Errorf("first element is not a map")
        }
        return nil, fmt.Errorf("empty response list")
    }

    // Sometimes APIs return objects directly, but python code says list.
    // If it's a map, maybe it's an error wrapper?
    if dataMap, ok := rawData.(map[string]interface{}); ok {
        // Check if it's the device object itself (unlikely based on python code) or has a data field
        // But python code treats root as list.
        // Let's print debug info if needed, but for now stick to python logic.
        return dataMap, nil // Fallback if it turns out to be a map
    }

	return nil, fmt.Errorf("unexpected response format")
}

// ParseDeviceStatus parses the raw API response into a standardized structure
func ParseDeviceStatus(deviceStatus map[string]interface{}, channel int) (*DeviceStatus, error) {
	if deviceStatus == nil {
		return nil, fmt.Errorf("device status is nil")
	}

	result := &DeviceStatus{
		Online: false,
		Output: false,
		Power:  0.0,
		Energy: 0.0,
	}

    // Online status
    if online, ok := deviceStatus["online"]; ok {
        // Could be float or bool in JSON unmarshal
        switch v := online.(type) {
        case bool:
            result.Online = v
        case float64:
            result.Online = v == 1
        }
    }

    statusObj, ok := deviceStatus["status"].(map[string]interface{})
    if !ok {
        // If no status object, return what we have (likely offline)
        return result, nil
    }

    channelKey := fmt.Sprintf("switch:%d", channel)

    // Helper function to safely get float
    getFloat := func(m map[string]interface{}, key string) (float64, bool) {
        if v, ok := m[key]; ok {
            if f, ok := v.(float64); ok {
                return f, true
            }
        }
        return 0, false
    }

    // Try switch:x
    if switchDataRaw, ok := statusObj[channelKey]; ok {
        if switchData, ok := switchDataRaw.(map[string]interface{}); ok {
            if out, ok := switchData["output"]; ok {
                 if b, ok := out.(bool); ok {
                     result.Output = b
                 }
            }

            if p, ok := getFloat(switchData, "apower"); ok {
                result.Power = p
            }

            if v, ok := getFloat(switchData, "voltage"); ok {
                val := v
                result.Voltage = &val
            }
            if c, ok := getFloat(switchData, "current"); ok {
                val := c
                result.Current = &val
            }

            if aenergyRaw, ok := switchData["aenergy"]; ok {
                if aenergy, ok := aenergyRaw.(map[string]interface{}); ok {
                    if t, ok := getFloat(aenergy, "total"); ok {
                        result.Energy = t
                    }
                }
            }

            if tempRaw, ok := switchData["temperature"]; ok {
                if temp, ok := tempRaw.(map[string]interface{}); ok {
                    if t, ok := getFloat(temp, "tC"); ok {
                        val := t
                        result.Temperature = &val
                    }
                }
            }
            return result, nil
        }
    }

    // Try Gen 1 relays/meters
    if relaysRaw, ok := statusObj["relays"]; ok {
        if relays, ok := relaysRaw.([]interface{}); ok {
            if channel < len(relays) {
                if relay, ok := relays[channel].(map[string]interface{}); ok {
                    if ison, ok := relay["ison"]; ok {
                         if b, ok := ison.(bool); ok {
                             result.Output = b
                         }
                    }
                }
            }
        }
    }

    if metersRaw, ok := statusObj["meters"]; ok {
        if meters, ok := metersRaw.([]interface{}); ok {
            if channel < len(meters) {
                if meter, ok := meters[channel].(map[string]interface{}); ok {
                    if p, ok := getFloat(meter, "power"); ok {
                        result.Power = p
                    }
                    if t, ok := getFloat(meter, "total"); ok {
                        result.Energy = t
                    }
                }
            }
        }
    }

    // Gen 1 temperature
    if tmpRaw, ok := statusObj["tmp"]; ok {
        if tmp, ok := tmpRaw.(map[string]interface{}); ok {
             if t, ok := getFloat(tmp, "tC"); ok {
                 val := t
                 result.Temperature = &val
             }
        }
    }

    // If we found relays/meters, we are done
    if _, ok := statusObj["relays"]; ok {
        return result, nil
    }

    // Try cover:0
    if coverRaw, ok := statusObj["cover:0"]; ok {
        if cover, ok := coverRaw.(map[string]interface{}); ok {
            if state, ok := cover["state"].(string); ok {
                result.Output = state == "open"
            }
            if p, ok := getFloat(cover, "apower"); ok {
                result.Power = p
            }
            return result, nil
        }
    }

    // Try light:0
    if lightRaw, ok := statusObj["light:0"]; ok {
        if light, ok := lightRaw.(map[string]interface{}); ok {
            if out, ok := light["output"]; ok {
                if b, ok := out.(bool); ok {
                    result.Output = b
                }
            }
            return result, nil
        }
    }

	return result, nil
}
