# Quick Start Guide

## Build and Run

```bash
make build
# or
go build -o burnafter ./cmd/burnafter
```

You can also run the binary directly:

```bash
go run ./cmd/burnafter
```

## Basic Usage

### Store a secret

```bash
# Store with default TTL (4 hours)
./burnafter store my-api-key "sk-1234567890abcdef"

# Store with custom TTL (2 hours = 7200 seconds)
./burnafter store github-token "ghp_xxxxxxxxxxxx" 7200
```

### Retrieve a secret

```bash
./burnafter get my-api-key
# Output: sk-1234567890abcdef
```

### Check server status

```bash
./burnafter ping
# Output: Server is alive
```

## Real-World Examples

### Store Database Password

```bash
# Store from password manager
./burnafter store db-password "$(pass show prod/database)"
```

### Use in Scripts

```bash
#!/bin/bash
# Store secret on first run
if ! ./burnafter ping &>/dev/null; then
    ./burnafter store api-key "$MY_SECRET_KEY" 14400
fi

# Use in your application
API_KEY=$(./burnafter get api-key)
curl -H "Authorization: Bearer $API_KEY" https://api.example.com/data
```

### CI/CD Pipeline Example

```bash
# In your CI job
./burnafter store deploy-token "$CI_DEPLOY_TOKEN" 3600

# Later in the pipeline
terraform apply -var "token=$(./burnafter get deploy-token)"
```

## Testing the Implementation

```bash
# Test store and retrieve
./burnafter store test "hello world" 60
./burnafter get test
# Output: hello world

# Verify server auto-start
pkill -f "burnafter.*server"
rm -f /tmp/burnafter.sock
./burnafter store auto-test "works!" 300
./burnafter get auto-test
# Output: works!
```

## Debug Mode

```bash
# Run with debug output
./burnafter -debug store secret-name "secret-value"

# Run server manually in foreground with debug
./burnafter -debug server
```

## Custom Socket Path

```bash
# Use a custom socket location
./burnafter -socket /tmp/my-custom.sock store key "value"
./burnafter -socket /tmp/my-custom.sock get key
```

## Security Features

### Binary Verification Test

This ensures that secrets can only be stored/read from the client application.

```bash
# Store a secret with the current binary
./burnafter store protected-secret "original"

# Try to access with a different binary (will fail)
cp burnafter burnafter-copy
./burnafter-copy get protected-secret
# Error: client binary hash mismatch - unauthorized

# Original binary still works
./burnafter get protected-secret
# Output: original
```

### Server Restart Behavior

Ensures that secrets are wiped out when the serve restarts.

```bash
# Store a secret
./burnafter store temp-secret "sensitive data"

# Kill and restart server (secret becomes unrecoverable)
pkill -f "burnafter.*server"
rm -f /tmp/burnafter.sock

# Try to retrieve (will fail - new server session ID)
./burnafter get temp-secret
# Error: secret not found

# Must store again
./burnafter store temp-secret "new data"
```

### Automatic Server Shutdown

The server automatically shuts down when:

- No activity for 10 minutes (inactivity timeout)
- All secrets have expired (no secrets remaining)

```bash
# Store a secret with short TTL
./burnafter store temp "data" 60  # 60 second TTL

# Server will automatically shut down after:
# 1. Secret expires (after 60 seconds of inactivity)
# 2. Cleanup cycle runs (every 60 seconds)
# 3. No secrets remain -> server shuts down

# Next operation will auto-start a new server
./burnafter store new-secret "data"
```

## Tips

1. **Multiple Secrets**: Each secret has its own name and TTL

   ```bash
   ./burnafter store github-token "ghp_xxx" 7200
   ./burnafter store gitlab-token "glpat_xxx" 7200
   ./burnafter store db-password "secret123" 3600
   ```

2. **Scripting**: Use command substitution to avoid exposing secrets

   ```bash
   export API_KEY=$(./burnafter get api-key)
   ```

3. **Pipe to Tools**: burnafter outputs only the secret value

   ```bash
   echo "password: $(./burnafter get db-pass)" | vault write secret/app -
   ```

4. **TTL Selection**:
   - Short tasks (< 1 hour): 3600 seconds
   - Development session: 14400 seconds (4 hours, default)
   - Build pipeline: 1800 seconds (30 minutes)
   - Maximum recommended: 28800 seconds (8 hours)
