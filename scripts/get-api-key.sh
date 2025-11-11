#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}📋 Octopus Energy API Key Helper${NC}"
echo ""
echo "This script will help you get your Octopus Energy API key."
echo ""

API_URL="https://octopus.energy/dashboard/new/accounts/personal-details/api-access"

echo -e "${BLUE}To get your API key:${NC}"
echo "  1. Visit: ${API_URL}"
echo "  2. Log in to your Octopus Energy account"
echo "  3. Navigate to: Account → Personal Details → API Access"
echo "  4. Generate or copy your API key"
echo "  5. Your account number is on your dashboard (format: A-XXXXXXXX)"
echo ""

# Try to open browser
read -p "Would you like to open this URL in your browser? (Y/n): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Nn]$ ]]; then
    echo -e "${GREEN}Opening browser...${NC}"
    
    # Detect OS and open browser
    if command -v xdg-open > /dev/null; then
        xdg-open "$API_URL"
    elif command -v open > /dev/null; then
        open "$API_URL"
    elif command -v start > /dev/null; then
        start "$API_URL"
    else
        echo -e "${YELLOW}Could not detect browser. Please open manually:${NC}"
        echo "$API_URL"
    fi
fi

echo ""
echo -e "${BLUE}After you have your API key:${NC}"
echo "  • Run: make configure"
echo "  • Or manually edit the .env file"
echo ""
