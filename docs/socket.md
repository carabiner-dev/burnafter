# Unix Socket Path Behavior

By default, `burnafter` generates a unique socket path based on the SHA256 hash of
the client binary:

```text
/tmp/burnafter-<first-16-chars-of-hash>.sock
```

This means:

- **Same binary = same socket**: Multiple copies of the same binary share the same server
- **Different binary = different socket**: Each unique binary version gets its own isolated server
- **Manual override**: Use `-socket` flag to specify a custom path

Note that two different client binaries can't share the same server,even with the
same socket as the digest verification fails.

Example:

```bash
# Uses auto-generated socket (e.g., /tmp/burnafter-f440959010e4ebf0.sock)
burnafter store api-key "secret"

# Uses custom socket path
burnafter -socket /tmp/my-app.sock store api-key "secret"
```
