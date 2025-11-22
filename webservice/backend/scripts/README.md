# Deployment Scripts

Scripts to help manage Sui token deployment and environment configuration.

## Scripts

### `parse_deploy_output.sh`

Parses the output from `TestDeployBridgeTokens` and extracts environment variables.

**Usage:**

```bash
# From test output file
./scripts/parse_deploy_output.sh test_output.txt

# From stdin
go test -run ^TestDeployBridgeTokens$ ... | ./scripts/parse_deploy_output.sh

# Or pipe from file
cat test_output.txt | ./scripts/parse_deploy_output.sh
```

**Output:**

```env
# Generated from TestDeployBridgeTokens output
# 2025-11-24 00:21:51

LFS_SUI_PACKAGE_ID=0xef72b2ea12711f6f74197e4f6c75caa9d06e4668a287bac9f059e334bc72b6bf
LFS_SUI_RPC_URL=https://fullnode.testnet.sui.io
LFS_SUI_OWNER=0x427e4cd7ed744ead2e0e83980b700fb3beb62f695902a556ce0f43f7c273365c
LFS_SUI_FTOKEN_TYPE=0xef72b2ea12711f6f74197e4f6c75caa9d06e4668a287bac9f059e334bc72b6bf::ftoken::FTOKEN<0x2::sui::SUI>
LFS_SUI_FTOKEN_TREASURY_CAP=0x3744ce291dc6317fa98efd0ff96906451f1e7c8544bd88648a2b1c164061a2e8
LFS_SUI_FTOKEN_AUTHORITY=0xeebe355882e3287ee85e7e400a91d14c937eb7c8fabaff266ae8e1c435028f41
LFS_SUI_XTOKEN_TYPE=0xef72b2ea12711f6f74197e4f6c75caa9d06e4668a287bac9f059e334bc72b6bf::xtoken::XTOKEN<0x2::sui::SUI>
LFS_SUI_XTOKEN_TREASURY_CAP=0xfc754f8ab1a8275f5408cfd8cde3241af3d587194c74762e89b27a19076f10f8
LFS_SUI_XTOKEN_AUTHORITY=0x8b30aa69a7a2390d5346d82c5c7ac649efce37352e2bebaf37381a0e706592df
```

### `deploy_and_update_env.sh`

All-in-one script that runs the deployment test and optionally updates your `.env` file.

**Usage:**

```bash
# Run from backend directory
./scripts/deploy_and_update_env.sh

# Or specify custom .env file location
ENV_FILE=/path/to/.env ./scripts/deploy_and_update_env.sh
```

**Features:**

- Runs `TestDeployBridgeTokens`
- Parses the deployment output
- Shows the extracted environment variables
- Prompts to update `.env` file
- Creates a backup of existing `.env` before updating
- Updates existing variables or appends new ones

## Workflow

### Manual deployment and parsing

```bash
# Run the deployment test
go test -timeout 30s -run ^TestDeployBridgeTokens$ github.com/leafsii/leafsii-backend/internal/api -v > deploy_output.txt

# Parse and output to .env format
./scripts/parse_deploy_output.sh deploy_output.txt > new_vars.env

# Review the output
cat new_vars.env

# Manually append to your .env if satisfied
cat new_vars.env >> .env
```

### Automated deployment and update

```bash
# Run the all-in-one script
./scripts/deploy_and_update_env.sh

# It will:
# 1. Run the test
# 2. Parse the output
# 3. Show you the variables
# 4. Ask if you want to update .env
# 5. Create a backup before updating
```

### Quick one-liner

```bash
# Run test and append to .env directly
go test -timeout 30s -run ^TestDeployBridgeTokens$ \
  github.com/leafsii/leafsii-backend/internal/api -v 2>&1 | \
  ./scripts/parse_deploy_output.sh >> .env
```

## Environment Variables Extracted

- `LFS_SUI_PACKAGE_ID` - The deployed Sui package ID
- `LFS_SUI_RPC_URL` - Sui RPC endpoint
- `LFS_SUI_OWNER` - Sui owner address (deployer/admin)
- `LFS_SUI_FTOKEN_TYPE` - Full coin type for FTOKEN
- `LFS_SUI_FTOKEN_TREASURY_CAP` - FTOKEN treasury cap object ID
- `LFS_SUI_FTOKEN_AUTHORITY` - FTOKEN mint authority object ID
- `LFS_SUI_XTOKEN_TYPE` - Full coin type for XTOKEN
- `LFS_SUI_XTOKEN_TREASURY_CAP` - XTOKEN treasury cap object ID
- `LFS_SUI_XTOKEN_AUTHORITY` - XTOKEN mint authority object ID

## Requirements

- Bash 4.0+ (for associative arrays)
- Standard Unix tools (grep, sed, awk)
