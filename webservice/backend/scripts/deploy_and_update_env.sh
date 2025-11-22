#!/bin/bash
# Runs TestDeployBridgeTokens and updates .env with the deployment information

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="${ENV_FILE:-$BACKEND_DIR/.env}"
PARSER="$SCRIPT_DIR/parse_deploy_output.sh"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Running TestDeployBridgeTokens...${NC}"

# Create a temp file for the test output
TEMP_OUTPUT=$(mktemp)
trap "rm -f $TEMP_OUTPUT" EXIT

# Run the test and capture output
cd "$BACKEND_DIR"
if go test -timeout 30s -run ^TestDeployBridgeTokens$ github.com/leafsii/leafsii-backend/internal/api -v 2>&1 | tee "$TEMP_OUTPUT"; then
    echo -e "${GREEN}Test passed!${NC}"
else
    echo -e "${RED}Test failed. Check output above.${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}Parsing deployment output...${NC}"

# Parse the output
PARSED_OUTPUT=$(bash "$PARSER" "$TEMP_OUTPUT")

if [ -z "$PARSED_OUTPUT" ]; then
    echo -e "${RED}Failed to parse deployment output. No variables extracted.${NC}"
    exit 1
fi

echo "$PARSED_OUTPUT"
echo ""

# Ask user if they want to update .env
read -p "Update $ENV_FILE with these values? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Backup existing .env
    if [ -f "$ENV_FILE" ]; then
        BACKUP_FILE="${ENV_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
        cp "$ENV_FILE" "$BACKUP_FILE"
        echo -e "${YELLOW}Backed up existing .env to $BACKUP_FILE${NC}"
    fi

    # Extract just the variable assignments (skip comments)
    echo "$PARSED_OUTPUT" | grep -E '^LFS_' > /tmp/new_vars.tmp

    # Update or append to .env file
    while IFS='=' read -r key value; do
        if [ -f "$ENV_FILE" ] && grep -q "^${key}=" "$ENV_FILE"; then
            # Update existing variable (macOS compatible)
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s|^${key}=.*|${key}=${value}|" "$ENV_FILE"
            else
                sed -i "s|^${key}=.*|${key}=${value}|" "$ENV_FILE"
            fi
            echo -e "${GREEN}Updated $key in $ENV_FILE${NC}"
        else
            # Append new variable
            echo "${key}=${value}" >> "$ENV_FILE"
            echo -e "${GREEN}Added $key to $ENV_FILE${NC}"
        fi
    done < /tmp/new_vars.tmp

    rm -f /tmp/new_vars.tmp
    echo -e "${GREEN}Successfully updated $ENV_FILE${NC}"
else
    echo "Skipped updating .env file"
fi
