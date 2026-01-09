# Security and System Architecture

The ephemeral storage of burnafter is handled by either a gRPC server launched
by the client library (server mode) or encrypted file-based storage (fallback
mode). The client library automatically selects the appropriate mode based on
server availability.

When a secret needs to be stored, the burnafter client library checks if the
server for the binary is running and if not attempts to start it. If server
startup fails or is disabled via `NoServer` option, the client transparently
falls back to encrypted file storage.

## Architecture

### Server Mode (Preferred)

```text
        ┌─────────────┐
        │   Client    │
        │  (Binary)   │
        └──────┬──────┘
            │ Unix Socket
            │ + gRPC
            │ + Peer Credentials
            ↓
        ┌─────────────┐
        │   Server    │
        │  (Daemon)   │
        ├─────────────┤
        │  Secrets    │
        │ Linux:      │
        │  → Kernel   │
        │    Keyring  │
        │ Others:     │
        │  → Memory   │
        └─────────────┘
```

**Characteristics:**

- **Linux**: Secrets stored in kernel keyring (never swapped to disk, process-isolated)
- **Others**: Secrets stored in memory only (never written to disk)
- Unix socket communication with peer credential verification
- Server auto-starts on first use
- Server auto-shuts down when all secrets expire or after inactivity timeout
- Binary hash-based socket paths prevent cross-binary access
- Strongest security guarantees

### Fallback Mode (Automatic)

```text
        ┌─────────────┐
        │   Client    │
        │  (Binary)   │
        └──────┬──────┘
               │
               │ Direct file I/O
               ↓
        ┌─────────────┐
        │  /tmp/...   │
        │ Encrypted   │
        │   Files     │
        │ (One per    │
        │  secret)    │
        └─────────────┘
```

**Characteristics:**

- Secrets encrypted with AES-256-GCM and stored as files in `/tmp`
- Automatic when server startup fails or `NoServer` option is set
- TTL enforcement via embedded expiry timestamps
- Deterministic file paths allow secret retrieval across invocations
- Weaker security than server mode (secrets persist on disk)

**Fallback Triggers:**

1. `NoServer` option explicitly set in client options
2. Server startup fails (permissions, SELinux, etc.)
3. Platform doesn't support Unix sockets (e.g., Windows in future)

## Security Model

### Server Mode Security

**Binary Isolation:**

- Socket path derived from binary SHA256 hash: `/tmp/burnafter-{hash[:16]}.sock`
- Different binary versions cannot access each other's secrets
- Recompiling with different nonce creates isolated secret store

**Authentication:**

- Unix socket peer credential verification (`SO_PEERCRED` on Linux, `LOCAL_PEERCRED` on macOS)
- Server validates:
  1. Client binary hash matches server's parent binary
  2. Client nonce matches server's compile-time nonce
  3. Client UID/PID from socket credentials

**Storage:**

- **Linux**: Secrets stored in kernel keyring (process-scoped, never swapped to disk)
- **Others**: Secrets encrypted in memory using AES-256-GCM
- Secrets never written to disk in either case
- Keys derived on-demand, never stored
- Server restart makes all secrets unrecoverable

**Key Derivation:**

```text
Key = AES256(
    input: session_id + client_nonce + binary_hash + secret_name
    salt:  binary_hash
    iterations: 100000 (PBKDF2-SHA256)
)
```

**Resource Limits:**

- Maximum 100 secrets per server (configurable via `MaxSecrets`)
- Maximum 1 MB per secret (configurable via `MaxSecretSize`)
- Inactivity timeout (default: disabled, configurable)

### Fallback Mode Security

**Encryption:**

- Algorithm: AES-256-GCM (authenticated encryption)
- Key derivation: PBKDF2-SHA256 with 100,000 iterations
- Random nonce per secret (12 bytes)
- Authentication tag prevents tampering

**Key Derivation:**

```go
Key = PBKDF2-SHA256(
    input: client_nonce + binary_hash + secret_name
    salt:  SHA256(secret_name)
    iterations: 100000
)
```

**File Format:**

```text
[version:1][nonce:12][expiry:8][ciphertext+tag:N]
│         │         │         └─ Encrypted secret + GCM tag
│         │         └─────────── Unix timestamp (absolute expiry)
│         └───────────────────── AES-GCM nonce (random)
└─────────────────────────────── File format version (1)
```

**File Paths:**

- Location: `/tmp/burnafter-{binary_hash[:16]}-{secret_hash[:16]}`
- Deterministic: Same binary + secret name = same file path
- Permissions: 0600 (owner read/write only)
- No file extension (opaque)

**TTL Enforcement:**

- Expiry timestamp embedded in encrypted file
- Checked on every `Get()` operation
- Expired files automatically deleted
- Background cleanup runs on each Store/Get/Delete

### Security Comparison

| Feature | Server Mode | Fallback Mode |
| --- | --- | --- |
| **Secret Storage** | Linux: Kernel keyring<br>Others: Memory only | Encrypted files in `/tmp` |
| **Persistence** | Lost on restart | Persists until TTL/OS cleanup |
| **Authentication** | Unix peer credentials | Binary hash + nonce |
| **Encryption** | AES-256-GCM | AES-256-GCM |
| **Key Derivation** | PBKDF2 (session-based) | PBKDF2 (deterministic) |
| **Cross-invocation** | ❌ No (new session) | ✅ Yes (same binary) |
| **Disk traces** | ❌ None | ⚠️ Files until TTL expires |
| **Platform support** | Linux, macOS | All platforms |
| **SELinux compatible** | ⚠️ May fail (memfd) | ✅ Yes |

## Security Considerations

### Common to Both Modes

1. **Binary Updates**: Updating the client binary invalidates all secrets (by design)
   - Binary hash changes, making keys underivable
   - New binary cannot decrypt secrets from old binary

2. **Same User Access**: Processes running as the same user can potentially access secrets
   - Server mode: Can attach debugger to server process memory
   - Fallback mode: Can read encrypted files (but need key)
   - Both modes: Still requires key derivation (nonce + binary hash)

3. **Key Derivation**: Encryption keys never stored; derived on-demand from:
   - Client nonce (compile-time constant)
   - Binary SHA256 hash
   - Secret name
   - Session ID (server mode only)

4. **Memory Attacks**:
   - Memory dumps may reveal secrets in both modes
   - Server mode (Linux): Secrets in kernel keyring (harder to extract than user-space memory)
   - Server mode (Others): Secrets in server process memory
   - Fallback mode: Secrets briefly in client process during encrypt/decrypt
   - Mitigation: Use memory locking if available, prefer Linux for kernel keyring protection

### Server Mode Specific

1. **Server Restart**: Restarting the server makes all secrets unrecoverable
   - Session ID is randomly generated on startup
   - Old session ID makes keys underivable

2. **Unix Socket Permissions**: Access controlled by filesystem permissions (0600)
   - Only owning user can connect to socket
   - Peer credentials provide additional verification

3. **Process Lifetime**: Server runs as daemon process
   - Detached from parent (Setsid)
   - Auto-shuts down when idle or no secrets remain
   - Can be manually killed: `pkill -f burnafter.*server`

### Fallback Mode Specific

1. **Disk Persistence**: Secrets persist as encrypted files
   - OS temp directory cleanup is eventual, not immediate
   - Files deleted automatically when TTL expires
   - Manual cleanup: `rm /tmp/burnafter-{hash}-*`

2. **File System Security**:
   - Relies on OS file permissions (0600)
   - Encrypted tmpfs is recommended for additional security
   - Consider using `shred` or secure deletion if needed

3. **Cross-Invocation Access**: Same binary can retrieve secrets across runs
    - Useful for CI/CD pipelines, scripts
    - Different from server mode's session-isolated storage

## Threat Model

### What burnafter PROTECTS Against

✅ Accidental secret exposure in logs/environment variables
✅ Secrets persisting after process termination (server mode)
✅ Cross-binary secret access (different applications)
✅ Unauthorized network access (local-only via Unix socket)
✅ Secret tampering (GCM authentication)
✅ Time-based secret expiry enforcement

### What burnafter DOES NOT protect against

❌ Root/admin access (can read any memory/file)
❌ Memory dumps of running processes
❌ Physical access to system (cold boot attacks)
❌ Malicious code in same process
❌ Side-channel attacks (timing, power analysis)
❌ Compromised binary (attacker can extract nonce)

## Best Practices

1. **Use Server Mode When Possible**: Provides strongest security guarantees

2. **Set Strong Nonces**: Use unique, random nonce for each application

   ```go
   opts := options.DefaultClient
   opts.Nonce = "random-unique-value-per-app-12345"
   ```

3. **Configure TTLs Appropriately**:
   - Short TTLs for high-security scenarios (minutes)
   - Longer TTLs for development/testing (hours)
   - Use absolute expiration for specific deadlines

4. **Monitor Server Lifecycle**:
   - Enable debug mode during development: `opts.Debug = true`
   - Check logs for startup failures
   - Verify fallback mode isn't being used unintentionally

5. **Secure Temp Directory**:
   - Use encrypted tmpfs for `/tmp` if possible
   - Consider setting `TMPDIR` to encrypted volume
   - Regularly clean old fallback files

6. **Binary Distribution**:
   - Sign binaries for macOS/Windows distribution
   - Notarize for macOS App Store distribution
   - Document nonce value for reproducible builds (if needed)

## Platform-Specific Notes

### Linux

- **Secret Storage**: Uses the kernel keyring (`KEY_SPEC_PROCESS_KEYRING`) for maximum security
  - Secrets stored in kernel space (never swapped to disk)
  - Process-isolated (automatic cleanup when server exits)
  - Falls back to memory storage if keyring unavailable
- **Server Execution**: Uses `memfd_create` for in-memory execution (when available)
  - Fallback to temp file if SELinux blocks memfd execution
- **Peer Credentials**: `SO_PEERCRED` socket option for client authentication

### macOS

- Server mode: Extracts server binary to `/tmp` (no memfd equivalent)
- Peer credentials: `LOCAL_PEERCRED` socket option

### Windows (Future)

- Server mode: Not yet supported (no Unix sockets)
- Fallback mode: Primary mechanism, much less secure as it is file-based
- Named pipes: Potential future server mode transport

## Cryptographic Details

### Algorithms

- **Symmetric Encryption**: AES-256-GCM
- **Key Derivation**: PBKDF2-HMAC-SHA256
- **Hashing**: SHA-256
- **Random Generation**: `crypto/rand` (Go stdlib)

### Parameters

- **PBKDF2 Iterations**: 100,000
- **AES Key Size**: 256 bits (32 bytes)
- **GCM Nonce Size**: 96 bits (12 bytes)
- **GCM Tag Size**: 128 bits (16 bytes)

### Key Derivation Inputs

**Server Mode:**

```go
sessionID (random 32 hex chars)
+ clientNonce (compile-time constant)
+ binaryHash (SHA256 of executable)
+ secretName (user-provided)
```

**Fallback Mode:**

```go
clientNonce (client defined constant)
+ binaryHash (SHA256 of executable)
+ secretName (user-provided)
```

Salt: `SHA256(secretName)` in fallback, `binaryHash` in server mode

### Random Number Generation

- Uses `crypto/rand.Reader` (cryptographically secure)
- GCM nonces are randomly generated per encryption
- Server session IDs are randomly generated on startup
- No PRNG seeding required (handled by OS)
