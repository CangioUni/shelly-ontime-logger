#!/usr/bin/env python3
"""
Shelly Cloud API Device Status Logger to InfluxDB
Polls Shelly devices via Shelly Cloud API and logs their on/off status
This version uses the official Shelly Cloud API to access devices remotely
"""

import requests
import time
import os
import yaml
from datetime import datetime
from influxdb_client import InfluxDBClient, Point
from influxdb_client.client.write_api import SYNCHRONOUS
import schedule
import logging

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class ShellyCloudStatusLogger:
    def __init__(self, config):
        self.config = config
        
        # Shelly Cloud API settings
        self.server_uri = config['shelly_cloud']['server_uri']
        self.auth_key = config['shelly_cloud']['auth_key']
        
        # Initialize InfluxDB client
        self.client = InfluxDBClient(
            url=config['influxdb']['url'],
            token=config['influxdb']['token'],
            org=config['influxdb']['org']
        )
        self.write_api = self.client.write_api(write_options=SYNCHRONOUS)
        self.default_bucket = config['influxdb']['bucket']
        self.devices = config['devices']
        
    def get_device_status_v2(self, device_id):
        """
        Get device status using Shelly Cloud API v2
        Returns device status or None if error
        """
        try:
            url = f"https://{self.server_uri}/v2/devices/api/get"
            
            payload = {
                "ids": [device_id],
                "select": ["status"]
            }
            
            response = requests.post(
                url,
                params={"auth_key": self.auth_key},
                json=payload,
                timeout=10
            )
            
            if response.status_code == 200:
                data = response.json()
                if data and len(data) > 0:
                    return data[0]
                else:
                    logger.warning(f"Empty response for device {device_id}")
                    return None
            else:
                logger.error(f"HTTP {response.status_code} for device {device_id}: {response.text}")
                return None
                
        except requests.exceptions.RequestException as e:
            logger.error(f"Error connecting to Shelly Cloud for {device_id}: {e}")
            return None
        except Exception as e:
            logger.error(f"Error parsing response for {device_id}: {e}")
            return None
    
    def parse_device_status(self, device_status, device_config):
        """
        Parse device status and extract relevant metrics
        Returns dict with standardized fields
        """
        if not device_status:
            return None
        
        result = {
            'online': device_status.get('online', 0) == 1,
            'output': False,
            'power': 0.0,
            'energy': 0.0,
            'voltage': None,
            'current': None,
            'temperature': None
        }
        
        # Get status object
        status = device_status.get('status', {})
        
        # Determine channel to read (default 0)
        channel = device_config.get('channel', 0)
        channel_key = f"switch:{channel}"
        
        # Try to get switch status
        if channel_key in status:
            switch_data = status[channel_key]
            result['output'] = switch_data.get('output', False)

            # Use safe float conversion
            apower = switch_data.get('apower')
            result['power'] = float(apower) if apower is not None else 0.0

            result['voltage'] = float(switch_data['voltage']) if switch_data.get('voltage') is not None else None
            result['current'] = float(switch_data['current']) if switch_data.get('current') is not None else None
            
            # Energy data
            aenergy = switch_data.get('aenergy', {})
            total_energy = aenergy.get('total')
            result['energy'] = float(total_energy) if total_energy is not None else 0.0
            
            # Temperature
            temp_data = switch_data.get('temperature', {})
            if temp_data:
                tC = temp_data.get('tC')
                result['temperature'] = float(tC) if tC is not None else None
        
        # Try Gen 1 relays/meters
        elif 'relays' in status:
            relays = status.get('relays', [])
            if channel < len(relays):
                relay_data = relays[channel]
                result['output'] = relay_data.get('ison', False)

            meters = status.get('meters', [])
            if channel < len(meters):
                meter_data = meters[channel]
                result['power'] = float(meter_data.get('power', 0.0))
                result['energy'] = float(meter_data.get('total', 0.0))

            # Gen 1 temperature
            if 'tmp' in status:
                tC = status['tmp'].get('tC')
                result['temperature'] = float(tC) if tC is not None else None

        # Try cover (roller) if switch not found
        elif 'cover:0' in status:
            cover_data = status['cover:0']
            # For covers, we consider "open" as ON
            result['output'] = cover_data.get('state', 'stopped') == 'open'
            result['power'] = float(cover_data.get('apower', 0.0))
        
        # Try light if neither switch nor cover found
        elif 'light:0' in status:
            light_data = status['light:0']
            result['output'] = light_data.get('output', False)
        
        return result
    
    def log_device_status(self, device):
        """Log status of a single device to InfluxDB"""
        device_id = device['id']
        device_name = device['name']
        
        # Get status from Shelly Cloud
        device_status = self.get_device_status_v2(device_id)
        
        bucket = device.get('bucket', self.default_bucket)

        if device_status is None:
            logger.warning(f"Failed to get status for {device_name} ({device_id})")
            # Log offline status
            point = Point("shelly_status") \
                .tag("device", device_name) \
                .tag("device_id", device_id) \
                .tag("type", device.get('type', 'unknown')) \
                .field("online", False) \
                .field("cloud_accessible", False) \
                .time(datetime.utcnow())
            
            self.write_api.write(bucket=bucket, record=point)
            return
        
        # Parse the status
        status = self.parse_device_status(device_status, device)
        
        if status is None:
            logger.warning(f"Failed to parse status for {device_name}")
            return
        
        # Create InfluxDB point
        point = Point("shelly_status") \
            .tag("device", device_name) \
            .tag("device_id", device_id) \
            .tag("type", device.get('type', 'unknown')) \
            .field("online", status['online']) \
            .field("cloud_accessible", True) \
            .field("output", status['output']) \
            .field("output_int", 1 if status['output'] else 0) \
            .field("power", status['power']) \
            .field("energy", status['energy'])
        
        # Add optional fields if available
        if status['voltage'] is not None:
            point.field("voltage", status['voltage'])
        if status['current'] is not None:
            point.field("current", status['current'])
        if status['temperature'] is not None:
            point.field("temperature", status['temperature'])
        
        point.time(datetime.utcnow())
        
        try:
            self.write_api.write(bucket=bucket, record=point)
            online_str = "online" if status['online'] else "offline"
            output_str = "ON" if status['output'] else "OFF"
            logger.info(f"✓ {device_name} ({online_str}): {output_str} ({status['power']:.1f}W)")
        except Exception as e:
            logger.error(f"Error writing to InfluxDB for {device_name}: {e}")
    
    def poll_all_devices(self):
        """Poll all configured devices"""
        logger.info(f"Polling {len(self.devices)} devices via Shelly Cloud API...")
        for device in self.devices:
            self.log_device_status(device)
            # Small delay to respect rate limits (1 req/sec)
            time.sleep(1.1)
        logger.info("Polling complete")
    
    def close(self):
        """Close InfluxDB connection"""
        self.client.close()


def load_config():
    """Load configuration from file or environment variables"""
    
    config_file = os.getenv('CONFIG_FILE', 'config.yaml')
    
    # Try to load from YAML file
    if os.path.exists(config_file):
        logger.info(f"Loading configuration from {config_file}")
        with open(config_file, 'r') as f:
            config = yaml.safe_load(f)
    else:
        logger.info("Config file not found, using defaults")
        config = {
            'shelly_cloud': {},
            'influxdb': {},
            'devices': [],
            'poll_interval': 5
        }
    
    # Defaults if missing in config file
    if 'server_uri' not in config.get('shelly_cloud', {}):
        config.setdefault('shelly_cloud', {})['server_uri'] = ''
    if 'auth_key' not in config.get('shelly_cloud', {}):
        config.setdefault('shelly_cloud', {})['auth_key'] = ''

    if 'url' not in config.get('influxdb', {}):
        config.setdefault('influxdb', {})['url'] = 'http://localhost:8086'
    if 'token' not in config.get('influxdb', {}):
        config.setdefault('influxdb', {})['token'] = ''
    if 'org' not in config.get('influxdb', {}):
        config.setdefault('influxdb', {})['org'] = ''
    if 'bucket' not in config.get('influxdb', {}):
        config.setdefault('influxdb', {})['bucket'] = 'shelly_status'
    
    env_poll_interval = os.getenv('POLL_INTERVAL')
    if env_poll_interval:
        config['poll_interval'] = int(env_poll_interval)
    elif 'poll_interval' not in config:
        config['poll_interval'] = 5
    
    # Handle groups if present and flatten to devices list
    if config.get('groups'):
        if not config.get('devices'):
            config['devices'] = []

        for group in config['groups']:
            group_bucket = group.get('bucket')
            for device in group.get('devices', []):
                # Copy the device dict to avoid modifying the original
                dev_config = device.copy()
                if group_bucket:
                    dev_config['bucket'] = group_bucket
                config['devices'].append(dev_config)

    return config


def main():
    logger.info("=== Starting Shelly Cloud API Status Logger ===")
    
    try:
        config = load_config()
    except Exception as e:
        logger.error(f"Failed to load configuration: {e}")
        return
    
    # Validate configuration
    if not config.get('devices'):
        logger.error("No devices configured! Please add devices or groups to config.yaml")
        return
    
    if not config['shelly_cloud'].get('auth_key'):
        logger.error("Shelly Cloud auth_key not configured!")
        logger.error("Get it from Shelly App > User Settings > Authorization cloud key")
        return
    
    if not config['shelly_cloud'].get('server_uri'):
        logger.error("Shelly Cloud server_uri not configured!")
        logger.error("Get it from Shelly App > User Settings > Authorization cloud key")
        return
    
    if not config['influxdb'].get('token'):
        logger.error("InfluxDB token not configured!")
        return
    
    logger.info(f"Monitoring {len(config['devices'])} devices")
    logger.info(f"Shelly Cloud: {config['shelly_cloud']['server_uri']}")
    logger.info(f"InfluxDB: {config['influxdb']['url']}, Bucket: {config['influxdb']['bucket']}")
    logger.info(f"Poll interval: {config['poll_interval']} minutes")
    logger.info("NOTE: Shelly Cloud API is rate-limited to 1 request/second")
    
    status_logger = ShellyCloudStatusLogger(config)
    
    # Schedule polling
    poll_interval = config['poll_interval']
    schedule.every(poll_interval).minutes.do(status_logger.poll_all_devices)
    
    # Do an initial poll immediately
    status_logger.poll_all_devices()
    
    try:
        while True:
            schedule.run_pending()
            time.sleep(30)  # Check schedule every 30 seconds
    except KeyboardInterrupt:
        logger.info("Shutting down...")
        status_logger.close()


if __name__ == "__main__":
    main()
