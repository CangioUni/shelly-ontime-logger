#!/usr/bin/env python3
"""
Helper script to list all Shelly devices from your Shelly Cloud account
Use this to get device IDs for your config.yaml
"""

import requests
import sys
import os

def get_all_devices(server_uri, auth_key):
    """
    Fetch all devices from Shelly Cloud account
    """
    try:
        url = f"https://{server_uri}/v2/devices/api/get"
        
        # Get device status for all devices
        # First, we need to get the list of device IDs
        # Using the devices_status endpoint from Real Time Events API
        status_url = f"https://{server_uri}/device/all_status"
        
        response = requests.post(
            status_url,
            data={"auth_key": auth_key},
            timeout=10
        )
        
        if response.status_code == 200:
            data = response.json()
            if data.get('isok'):
                devices_status = data.get('data', {}).get('devices_status', {})
                return devices_status
            else:
                print(f"Error: API returned isok=false")
                return None
        else:
            print(f"HTTP Error {response.status_code}: {response.text}")
            return None
            
    except requests.exceptions.RequestException as e:
        print(f"Error connecting to Shelly Cloud: {e}")
        return None
    except Exception as e:
        print(f"Error: {e}")
        return None


def print_device_info(devices_status):
    """
    Print device information in a formatted way
    """
    if not devices_status:
        print("No devices found or error occurred")
        return
    
    print("\n" + "="*80)
    print("YOUR SHELLY DEVICES")
    print("="*80)
    print(f"Total devices: {len(devices_status)}\n")
    
    for device_id, device_data in devices_status.items():
        dev_info = device_data.get('_dev_info', {})
        
        print(f"Device ID: {device_id}")
        print(f"  Model: {dev_info.get('code', 'Unknown')}")
        print(f"  Generation: {dev_info.get('gen', 'Unknown')}")
        print(f"  Online: {dev_info.get('online', False)}")
        
        # Try to get device name from status
        status_data = {k: v for k, v in device_data.items() if not k.startswith('_')}
        if status_data:
            print(f"  Components: {', '.join(status_data.keys())}")
        
        print()
    
    print("="*80)
    print("\nCopy the Device IDs above into your config_cloud.yaml")
    print("Example:")
    print("""
devices:
  - name: "my_device"
    id: "{device_id}"
    type: "plus1pm"
    channel: 0
""")


def main():
    print("Shelly Cloud Device Lister")
    print("-" * 40)
    
    # Get credentials from environment or prompt
    server_uri = os.getenv('SHELLY_SERVER_URI')
    auth_key = os.getenv('SHELLY_AUTH_KEY')
    
    if not server_uri:
        server_uri = input("Enter your Shelly Cloud server URI (e.g., shelly-103-eu.shelly.cloud): ").strip()
    
    if not auth_key:
        auth_key = input("Enter your Shelly Cloud auth key: ").strip()
    
    if not server_uri or not auth_key:
        print("Error: Server URI and auth key are required")
        sys.exit(1)
    
    print(f"\nConnecting to {server_uri}...")
    
    devices = get_all_devices(server_uri, auth_key)
    
    if devices:
        print_device_info(devices)
    else:
        print("\nFailed to retrieve devices. Please check:")
        print("1. Server URI is correct")
        print("2. Auth key is valid")
        print("3. You have devices connected to Shelly Cloud")
        sys.exit(1)


if __name__ == "__main__":
    main()
