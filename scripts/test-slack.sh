#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}🔔 Testing Slack Webhook Connection${NC}"
echo ""

# Load environment variables
if [ ! -f .env ]; then
    echo -e "${RED}✗ .env file not found${NC}"
    echo "Please run 'make configure' first"
    exit 1
fi

source .env

if [ -z "$SLACK_WEBHOOK_URL" ]; then
    echo -e "${RED}✗ SLACK_WEBHOOK_URL not set${NC}"
    echo "Please run 'make configure' to set up Slack"
    exit 1
fi

echo -e "${BLUE}Webhook URL: ${SLACK_WEBHOOK_URL:0:50}...${NC}"
echo ""

# Create test message payload
PAYLOAD=$(cat <<EOF
{
    "attachments": [
        {
            "color": "good",
            "title": "🧪 Test Message from Octopus Home Mini Monitor",
            "text": "If you can see this message, your Slack webhook is configured correctly!",
            "fields": [
                {
                    "title": "Status",
                    "value": "Configuration Test",
                    "short": true
                },
                {
                    "title": "Time",
                    "value": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
                    "short": true
                }
            ],
            "footer": "Octopus Home Mini Monitor",
            "ts": $(date +%s)
        }
    ]
}
EOF
)

# Send test message
echo -e "${CYAN}Sending test message...${NC}"
HTTP_CODE=$(curl -s -o /tmp/slack_response.txt -w "%{http_code}" \
    -X POST \
    -H 'Content-type: application/json' \
    --data "$PAYLOAD" \
    "$SLACK_WEBHOOK_URL")

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ Success!${NC}"
    echo ""
    echo "Check your Slack channel for the test message."
    echo -e "${GREEN}Slack webhook is configured correctly!${NC}"
    exit 0
else
    echo -e "${RED}✗ Failed${NC}"
    echo ""
    echo "HTTP Status Code: $HTTP_CODE"
    echo "Response:"
    cat /tmp/slack_response.txt
    echo ""
    echo -e "${RED}Slack webhook test failed${NC}"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Verify the webhook URL is correct"
    echo "  2. Ensure the webhook is still active in Slack"
    echo "  3. Check that the app has permission to post to the channel"
    exit 1
fi
