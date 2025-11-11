#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}💾 Testing InfluxDB Connection${NC}"
echo ""

# Load environment variables
if [ ! -f .env ]; then
    echo -e "${RED}✗ .env file not found${NC}"
    echo "Please run 'make configure' first"
    exit 1
fi

source .env

if [ -z "$INFLUXDB_URL" ] || [ -z "$INFLUXDB_TOKEN" ] || [ -z "$INFLUXDB_ORG" ]; then
    echo -e "${RED}✗ InfluxDB configuration incomplete${NC}"
    echo "Please run 'make configure' to set up InfluxDB"
    exit 1
fi

echo -e "${BLUE}InfluxDB URL: ${INFLUXDB_URL}${NC}"
echo -e "${BLUE}Organization: ${INFLUXDB_ORG}${NC}"
echo -e "${BLUE}Bucket: ${INFLUXDB_BUCKET}${NC}"
echo ""

# Test health endpoint
echo -e "${CYAN}Testing health endpoint...${NC}"
HTTP_CODE=$(curl -s -o /tmp/influx_health.txt -w "%{http_code}" \
    "${INFLUXDB_URL}/health")

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ InfluxDB is reachable${NC}"

    # Check health status
    STATUS=$(cat /tmp/influx_health.txt | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
    if [ "$STATUS" = "pass" ]; then
        echo -e "${GREEN}✓ InfluxDB health check: pass${NC}"
    else
        echo -e "${YELLOW}⚠ InfluxDB health check: $STATUS${NC}"
    fi
else
    echo -e "${RED}✗ Cannot reach InfluxDB${NC}"
    echo "HTTP Status Code: $HTTP_CODE"
    echo "Response:"
    cat /tmp/influx_health.txt
    echo ""
    exit 1
fi

echo ""

# Test authentication
echo -e "${CYAN}Testing authentication...${NC}"
HTTP_CODE=$(curl -s -o /tmp/influx_auth.txt -w "%{http_code}" \
    -H "Authorization: Token ${INFLUXDB_TOKEN}" \
    "${INFLUXDB_URL}/api/v2/buckets?org=${INFLUXDB_ORG}")

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ Authentication successful${NC}"
else
    echo -e "${RED}✗ Authentication failed${NC}"
    echo "HTTP Status Code: $HTTP_CODE"
    echo "Response:"
    cat /tmp/influx_auth.txt
    echo ""
    echo "Please check your INFLUXDB_TOKEN"
    exit 1
fi

echo ""

# Check if bucket exists
echo -e "${CYAN}Checking bucket '${INFLUXDB_BUCKET}'...${NC}"
BUCKET_EXISTS=$(cat /tmp/influx_auth.txt | grep -o "\"name\":\"${INFLUXDB_BUCKET}\"" || echo "")

if [ -n "$BUCKET_EXISTS" ]; then
    echo -e "${GREEN}✓ Bucket '${INFLUXDB_BUCKET}' exists${NC}"
else
    echo -e "${YELLOW}⚠ Bucket '${INFLUXDB_BUCKET}' not found${NC}"
    echo ""
    echo "To create the bucket, run:"
    echo "  influx bucket create -n ${INFLUXDB_BUCKET} -o ${INFLUXDB_ORG}"
    echo ""
    echo "Or create it via the InfluxDB UI:"
    echo "  ${INFLUXDB_URL}"
    exit 1
fi

echo ""

# Test write permission by writing a test point
echo -e "${CYAN}Testing write permission...${NC}"

TEST_DATA="test_connection,source=setup_script value=1 $(date +%s)000000000"

HTTP_CODE=$(curl -s -o /tmp/influx_write.txt -w "%{http_code}" \
    -X POST \
    -H "Authorization: Token ${INFLUXDB_TOKEN}" \
    -H "Content-Type: text/plain; charset=utf-8" \
    --data-binary "$TEST_DATA" \
    "${INFLUXDB_URL}/api/v2/write?org=${INFLUXDB_ORG}&bucket=${INFLUXDB_BUCKET}&precision=ns")

if [ "$HTTP_CODE" = "204" ]; then
    echo -e "${GREEN}✓ Write permission confirmed${NC}"
else
    echo -e "${RED}✗ Write test failed${NC}"
    echo "HTTP Status Code: $HTTP_CODE"
    echo "Response:"
    cat /tmp/influx_write.txt
    echo ""
    echo "The token may not have write permission to the bucket"
    exit 1
fi

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✓ All InfluxDB tests passed!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Your InfluxDB configuration is correct and ready to use."
