#!/bin/bash
# Script to parse TestDeployBridgeTokens output and generate .env entries
# Usage:
#   ./parse_deploy_output.sh < test_output.txt
#   ./parse_deploy_output.sh test_output.txt
#   go test ... | ./parse_deploy_output.sh

set -euo pipefail

# Read from file or stdin
if [ $# -gt 0 ]; then
    input="$1"
else
    input="/dev/stdin"
fi

# Temporary associative array simulation using variables
declare -A env_vars

# Parse the output
while IFS= read -r line; do
    # Extract Package ID
    if [[ "$line" =~ Package=([0-9a-fx]+) ]]; then
        env_vars[LFS_SUI_PACKAGE_ID]="${BASH_REMATCH[1]}"
    fi

    # Extract FTOKEN info
    if [[ "$line" =~ FTOKEN:.*coinType=([^[:space:]]+).*treasuryCap=([^[:space:]]+).*mintAuthority=([^[:space:]]+) ]]; then
        env_vars[LFS_SUI_FTOKEN_TYPE]="${BASH_REMATCH[1]}"
        env_vars[LFS_SUI_FTOKEN_TREASURY_CAP]="${BASH_REMATCH[2]}"
        env_vars[LFS_SUI_FTOKEN_AUTHORITY]="${BASH_REMATCH[3]}"
    fi

    # Extract XTOKEN info
    if [[ "$line" =~ XTOKEN:.*coinType=([^[:space:]]+).*treasuryCap=([^[:space:]]+).*mintAuthority=([^[:space:]]+) ]]; then
        env_vars[LFS_SUI_XTOKEN_TYPE]="${BASH_REMATCH[1]}"
        env_vars[LFS_SUI_XTOKEN_TREASURY_CAP]="${BASH_REMATCH[2]}"
        env_vars[LFS_SUI_XTOKEN_AUTHORITY]="${BASH_REMATCH[3]}"
    fi

    # Extract RPC URL
    if [[ "$line" =~ LFS_SUI_RPC_URL=([^[:space:]]+) ]]; then
        env_vars[LFS_SUI_RPC_URL]="${BASH_REMATCH[1]}"
    fi

    # Extract SUI_OWNER
    if [[ "$line" =~ LFS_SUI_OWNER=(0x[0-9a-f]+) ]]; then
        env_vars[LFS_SUI_OWNER]="${BASH_REMATCH[1]}"
    fi
done < "$input"

# Print .env format with header
echo "# Generated from TestDeployBridgeTokens output"
echo "# $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# Print in specific order for readability
keys=(
    "LFS_SUI_PACKAGE_ID"
    "LFS_SUI_RPC_URL"
    "LFS_SUI_OWNER"
    "LFS_SUI_FTOKEN_TYPE"
    "LFS_SUI_FTOKEN_TREASURY_CAP"
    "LFS_SUI_FTOKEN_AUTHORITY"
    "LFS_SUI_XTOKEN_TYPE"
    "LFS_SUI_XTOKEN_TREASURY_CAP"
    "LFS_SUI_XTOKEN_AUTHORITY"
)

for key in "${keys[@]}"; do
    if [ -n "${env_vars[$key]:-}" ]; then
        echo "${key}=${env_vars[$key]}"
    fi
done
