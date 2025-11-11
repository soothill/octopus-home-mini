#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}🔍 Verifying Configuration${NC}"
echo ""

ERRORS=0
WARNINGS=0

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${RED}✗ .env file not found${NC}"
    echo "Please run 'make configure' first"
    exit 1
fi

source .env

# Function to check required variable
check_required() {
    local var_name="$1"
    local var_value="$2"
    local description="$3"

    if [ -z "$var_value" ]; then
        echo -e "${RED}✗ $var_name is not set${NC}"
        echo "  $description"
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}✓ $var_name is set${NC}"
    fi
}

# Function to check optional variable
check_optional() {
    local var_name="$1"
    local var_value="$2"
    local default_value="$3"

    if [ -z "$var_value" ]; then
        echo -e "${YELLOW}⚠ $var_name is not set (will use default: $default_value)${NC}"
        WARNINGS=$((WARNINGS + 1))
    else
        echo -e "${GREEN}✓ $var_name is set${NC}"
    fi
}

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Octopus Energy Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
check_required "OCTOPUS_API_KEY" "$OCTOPUS_API_KEY" "Get it from: https://octopus.energy/dashboard"
check_required "OCTOPUS_ACCOUNT_NUMBER" "$OCTOPUS_ACCOUNT_NUMBER" "Format: A-XXXXXXXX"
echo ""

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}InfluxDB Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
check_required "INFLUXDB_URL" "$INFLUXDB_URL" "e.g., http://localhost:8086"
check_required "INFLUXDB_TOKEN" "$INFLUXDB_TOKEN" "Generate in InfluxDB UI"
check_required "INFLUXDB_ORG" "$INFLUXDB_ORG" "Your InfluxDB organization name"
check_required "INFLUXDB_BUCKET" "$INFLUXDB_BUCKET" "Bucket to store data"
echo ""

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Slack Configuration${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
check_required "SLACK_WEBHOOK_URL" "$SLACK_WEBHOOK_URL" "Get it from: https://api.slack.com/apps"
echo ""

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Application Settings${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
check_optional "POLL_INTERVAL_SECONDS" "$POLL_INTERVAL_SECONDS" "30"
check_optional "CACHE_DIR" "$CACHE_DIR" "./cache"
check_optional "LOG_LEVEL" "$LOG_LEVEL" "info"
echo ""

# Validate specific values
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Validation Checks${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Check account number format
if [[ ! "$OCTOPUS_ACCOUNT_NUMBER" =~ ^A-[0-9A-F]{8}$ ]] && [ -n "$OCTOPUS_ACCOUNT_NUMBER" ]; then
    echo -e "${YELLOW}⚠ Account number format looks incorrect (expected: A-XXXXXXXX)${NC}"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✓ Account number format looks valid${NC}"
fi

# Check URL format
if [[ ! "$INFLUXDB_URL" =~ ^https?:// ]] && [ -n "$INFLUXDB_URL" ]; then
    echo -e "${YELLOW}⚠ InfluxDB URL should start with http:// or https://${NC}"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✓ InfluxDB URL format looks valid${NC}"
fi

# Check Slack webhook URL format
if [[ ! "$SLACK_WEBHOOK_URL" =~ ^https://hooks.slack.com/services/ ]] && [ -n "$SLACK_WEBHOOK_URL" ]; then
    echo -e "${YELLOW}⚠ Slack webhook URL format looks incorrect${NC}"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✓ Slack webhook URL format looks valid${NC}"
fi

# Check poll interval is a number
if ! [[ "$POLL_INTERVAL_SECONDS" =~ ^[0-9]+$ ]] && [ -n "$POLL_INTERVAL_SECONDS" ]; then
    echo -e "${RED}✗ POLL_INTERVAL_SECONDS must be a number${NC}"
    ERRORS=$((ERRORS + 1))
else
    echo -e "${GREEN}✓ Poll interval is valid${NC}"

    # Warn if poll interval is too low
    if [ -n "$POLL_INTERVAL_SECONDS" ] && [ "$POLL_INTERVAL_SECONDS" -lt 10 ]; then
        echo -e "${YELLOW}⚠ Poll interval < 10s may cause API rate limit issues${NC}"
        WARNINGS=$((WARNINGS + 1))
    fi
fi

# Check log level
if [ -n "$LOG_LEVEL" ] && [[ ! "$LOG_LEVEL" =~ ^(debug|info|warn|error)$ ]]; then
    echo -e "${YELLOW}⚠ LOG_LEVEL should be: debug, info, warn, or error${NC}"
    WARNINGS=$((WARNINGS + 1))
fi

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Summary${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    echo -e "${GREEN}✓ All checks passed!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Test Slack: make test-slack"
    echo "  2. Test InfluxDB: make test-influx"
    echo "  3. Build: make build"
    echo "  4. Run: make run"
elif [ $ERRORS -eq 0 ]; then
    echo -e "${YELLOW}Configuration complete with $WARNINGS warning(s)${NC}"
    echo ""
    echo "You can proceed, but consider addressing the warnings above."
else
    echo -e "${RED}Configuration has $ERRORS error(s) and $WARNINGS warning(s)${NC}"
    echo ""
    echo "Please run 'make configure' to fix the issues."
    exit 1
fi
