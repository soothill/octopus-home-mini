#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}Octopus Home Mini Monitor Setup${NC}"
echo -e "${BLUE}================================${NC}"
echo ""

# Check if .env exists
if [ -f .env ]; then
    echo -e "${YELLOW}⚠ .env file already exists${NC}"
    read -p "Do you want to reconfigure? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${GREEN}✓ Using existing configuration${NC}"
        exit 0
    fi
    # Backup existing .env
    cp .env .env.backup
    echo -e "${GREEN}✓ Backed up existing .env to .env.backup${NC}"
fi

# Copy example file
cp .env.example .env
echo -e "${GREEN}✓ Created .env file${NC}"
echo ""

# Run configuration wizard
bash scripts/configure.sh

echo ""
echo -e "${BLUE}================================${NC}"
echo -e "${GREEN}✓ Setup complete!${NC}"
echo -e "${BLUE}================================${NC}"
echo ""
echo "Next steps:"
echo "  1. Review your configuration: cat .env"
echo "  2. Test Slack webhook: make test-slack"
echo "  3. Test InfluxDB connection: make test-influx"
echo "  4. Build the application: make build"
echo "  5. Run the monitor: make run"
echo ""
