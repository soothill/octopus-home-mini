#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}📋 Configuration Wizard${NC}"
echo ""
echo "This wizard will help you configure the Octopus Home Mini Monitor."
echo "Press Enter to keep the default value shown in [brackets]."
echo ""

# Load existing values if .env exists
if [ -f .env ]; then
    source .env
fi

# Function to read input with default
read_with_default() {
    local prompt="$1"
    local default="$2"
    local var_name="$3"
    local secret="${4:-false}"

    if [ "$secret" = "true" ]; then
        read -s -p "$(echo -e ${prompt})" value
        echo ""
    else
        read -p "$(echo -e ${prompt})" value
    fi

    if [ -z "$value" ]; then
        value="$default"
    fi

    eval "$var_name='$value'"
}

# Octopus Energy Configuration
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}1. Octopus Energy API Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "To get your API key:"
echo "  1. Visit: https://octopus.energy/dashboard/new/accounts/personal-details/api-access"
echo "  2. Generate or copy your API key"
echo ""

read_with_default "Octopus API Key [${OCTOPUS_API_KEY}]: " "${OCTOPUS_API_KEY}" OCTOPUS_API_KEY true
echo ""

echo "Your account number is displayed on your dashboard (format: A-XXXXXXXX)"
read_with_default "Octopus Account Number [${OCTOPUS_ACCOUNT_NUMBER}]: " "${OCTOPUS_ACCOUNT_NUMBER}" OCTOPUS_ACCOUNT_NUMBER
echo ""

# InfluxDB Configuration
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}2. InfluxDB Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "InfluxDB stores your energy consumption data for analysis."
echo ""

read_with_default "InfluxDB URL [${INFLUXDB_URL:-http://localhost:8086}]: " "${INFLUXDB_URL:-http://localhost:8086}" INFLUXDB_URL
read_with_default "InfluxDB Token [${INFLUXDB_TOKEN}]: " "${INFLUXDB_TOKEN}" INFLUXDB_TOKEN true
echo ""
read_with_default "InfluxDB Organization [${INFLUXDB_ORG}]: " "${INFLUXDB_ORG}" INFLUXDB_ORG
read_with_default "InfluxDB Bucket [${INFLUXDB_BUCKET:-octopus_energy}]: " "${INFLUXDB_BUCKET:-octopus_energy}" INFLUXDB_BUCKET
echo ""

# Slack Configuration
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}3. Slack Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Slack notifications alert you of failures and important events."
echo ""
echo "To create a webhook:"
echo "  1. Visit: https://api.slack.com/apps"
echo "  2. Create a new app or use an existing one"
echo "  3. Enable Incoming Webhooks"
echo "  4. Create a webhook for your desired channel"
echo ""

read_with_default "Slack Webhook URL [${SLACK_WEBHOOK_URL}]: " "${SLACK_WEBHOOK_URL}" SLACK_WEBHOOK_URL
echo ""

# Application Settings
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}4. Application Settings${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

read_with_default "Poll Interval (seconds) [${POLL_INTERVAL_SECONDS:-30}]: " "${POLL_INTERVAL_SECONDS:-30}" POLL_INTERVAL_SECONDS
read_with_default "Cache Directory [${CACHE_DIR:-./cache}]: " "${CACHE_DIR:-./cache}" CACHE_DIR
read_with_default "Log Level (info/debug/error) [${LOG_LEVEL:-info}]: " "${LOG_LEVEL:-info}" LOG_LEVEL
echo ""

# Write configuration to .env
echo -e "${CYAN}💾 Saving configuration...${NC}"

cat > .env << EOF
# Octopus Energy API Configuration
OCTOPUS_API_KEY=${OCTOPUS_API_KEY}
OCTOPUS_ACCOUNT_NUMBER=${OCTOPUS_ACCOUNT_NUMBER}

# InfluxDB Configuration
INFLUXDB_URL=${INFLUXDB_URL}
INFLUXDB_TOKEN=${INFLUXDB_TOKEN}
INFLUXDB_ORG=${INFLUXDB_ORG}
INFLUXDB_BUCKET=${INFLUXDB_BUCKET}

# Slack Configuration
SLACK_WEBHOOK_URL=${SLACK_WEBHOOK_URL}

# Application Configuration
POLL_INTERVAL_SECONDS=${POLL_INTERVAL_SECONDS}
CACHE_DIR=${CACHE_DIR}
LOG_LEVEL=${LOG_LEVEL}
EOF

echo -e "${GREEN}✓ Configuration saved to .env${NC}"
echo ""

# Offer to test connections
echo -e "${CYAN}Would you like to test your configuration now?${NC}"
read -p "Test Slack webhook? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    bash scripts/test-slack.sh
fi

read -p "Test InfluxDB connection? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    bash scripts/test-influx.sh
fi

echo ""
echo -e "${GREEN}✓ Configuration complete!${NC}"
