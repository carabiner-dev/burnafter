// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package secrets

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/chainguard-dev/clog"
	"golang.org/x/sys/unix"

	"github.com/carabiner-dev/burnafter/secrets"
)

// Ensure the driver implements the storage interface
var _ secrets.Storage = &KeyringStorage{}

// keyringRequest represents a request to perform a keyring operation
type keyringRequest struct {
	op       string // "store", "get", "delete"
	id       string
	payload  *secrets.Payload
	respChan chan keyringResponse
}

// keyringResponse represents the response from a keyring operation
type keyringResponse struct {
	payload *secrets.Payload
	err     error
}

// Global worker state - shared across all KeyringStorage instances
var (
	workerOnce    sync.Once
	workerReqChan chan keyringRequest
	errWorkerInit error
	workerKeyring int
)

// KeyringStorage is a Linux kernel keyring implementation of the secrets.Storage
// interface. It stores encrypted secrets in the process keyring.
//
// This driver uses the KEY_SPEC_PROCESS_KEYRING key ring, meaning no other
// program, even from the same user, can read the encrypted secrets.
//
// Unfortunately. After much testing, the process-scoped keyring seems impossible
// to access from child threads when the GRPC server is processing requests. To
// get around this, all operations are dispatched to a shared worker goroutine
// locked to a OS thread. Multiple instances share the same worker to ensure
// consistent keyring access which should never happen outside of tests.
type KeyringStorage struct{}

// initWorker initializes the global keyring worker goroutine (called once)
func initWorker(ctx context.Context) {
	workerReqChan = make(chan keyringRequest)
	initDone := make(chan error, 1)

	// Start worker goroutine locked to an OS thread
	go func() {
		// Lock this goroutine to its OS thread - required for thread-local keyring
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Initialize the process keyring on this thread
		keyringId, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_PROCESS_KEYRING, true)
		if err != nil {
			initDone <- fmt.Errorf("failed to access/create process keyring: %w", err)
			return
		}
		workerKeyring = keyringId

		clog.FromContext(ctx).Debugf("Process keyring initialized: keyring=%d pid=%d tid=%d uid=%d euid=%d",
			workerKeyring, os.Getpid(), unix.Gettid(), os.Getuid(), os.Geteuid())

		// Set permissions on the keyring
		if err = unix.KeyctlSetperm(workerKeyring, 0x3f3f0000); err != nil {
			initDone <- fmt.Errorf("failed to set keyring permissions: %w", err)
			return
		}

		// Signal successful initialization
		initDone <- nil

		// Process requests on this locked thread
		for req := range workerReqChan {
			switch req.op {
			case "store":
				req.respChan <- keyringResponse{err: storeOnThread(ctx, workerKeyring, req.id, req.payload)}
			case "get":
				payload, err := getOnThread(ctx, workerKeyring, req.id)
				req.respChan <- keyringResponse{payload: payload, err: err}
			case "delete":
				req.respChan <- keyringResponse{err: deleteOnThread(ctx, workerKeyring, req.id)}
			}
		}
	}()

	// Wait for initialization to complete
	errWorkerInit = <-initDone
	if errWorkerInit != nil {
		close(workerReqChan)
	}
}

// NewKeyringStorage creates a new kernel keyring storage backend.
// It uses the process keyring (KEY_SPEC_PROCESS_KEYRING) which does not seem to
// be accessible from other threads outside of the main one (when threadid == pid).
//
// To handle this, a shared worker goroutine locked to an OS thread handles
// all keyring operations, ensuring all calls come from the same thread.
// Multiple KeyringStorage instances share the same worker.
func NewKeyringStorage(ctx context.Context) (*KeyringStorage, error) {
	// Initialize the global worker exactly once
	workerOnce.Do(func() {
		initWorker(ctx)
	})

	// Return error if worker failed to initialize
	if errWorkerInit != nil {
		return nil, errWorkerInit
	}

	return &KeyringStorage{}, nil
}

// Store persists a secret in the kernel keyring.
func (k *KeyringStorage) Store(ctx context.Context, id string, secret *secrets.Payload) error {
	clog.FromContext(ctx).With("keyring", workerKeyring).
		With("pid", os.Getpid()).With("tid", unix.Gettid()).
		Debugf("Dispatching store for secret %s", id)

	respChan := make(chan keyringResponse, 1)
	workerReqChan <- keyringRequest{
		op:       "store",
		id:       id,
		payload:  secret,
		respChan: respChan,
	}

	resp := <-respChan
	return resp.err
}

// storeOnThread performs the actual store operation on the locked thread
func storeOnThread(ctx context.Context, keyringId int, id string, secret *secrets.Payload) error {
	clog.FromContext(ctx).With("keyring", keyringId).
		With("pid", os.Getpid()).With("tid", unix.Gettid()).
		Debugf("Storing secret %s on locked thread", id)

	// Serialize the payload to bytes using gob encoding
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(secret); err != nil {
		return fmt.Errorf("failed to encode the secret payload: %w", err)
	}

	// Check if key already exists and delete it first
	if existingKeyID, err := unix.KeyctlSearch(keyringId, "user", id, 0); err == nil {
		//nolint:errcheck // Don't err if key can't be removed it will be overwritten anyway.
		_, _ = unix.KeyctlInt(unix.KEYCTL_UNLINK, existingKeyID, keyringId, 0, 0)
	}

	// Add the key to the keyring
	_, err := unix.AddKey("user", id, buf.Bytes(), keyringId)
	if err != nil {
		return fmt.Errorf("adding key to keyring %d: %w", keyringId, err)
	}

	return nil
}

// Get retrieves a secret from the kernel keyring by its ID.
func (k *KeyringStorage) Get(ctx context.Context, id string) (*secrets.Payload, error) {
	clog.FromContext(ctx).Debugf("Dispatching get for secret %s (pid=%d, tid=%d)", id, os.Getpid(), unix.Gettid())

	respChan := make(chan keyringResponse, 1)
	workerReqChan <- keyringRequest{
		op:       "get",
		id:       id,
		respChan: respChan,
	}

	resp := <-respChan
	return resp.payload, resp.err
}

// getOnThread performs the actual get operation on the locked thread
func getOnThread(ctx context.Context, keyringId int, id string) (*secrets.Payload, error) {
	clog.FromContext(ctx).Debugf("Getting secret %s from keyring %d (pid=%d, tid=%d)", id, keyringId, os.Getpid(), unix.Gettid())

	// Search for the key by description
	keyID, err := unix.KeyctlSearch(keyringId, "user", id, 0)
	if err != nil {
		return nil, fmt.Errorf("looking up secret in keyring %d: %w", keyringId, err)
	}

	// First, get the size of the key data
	size, err := unix.KeyctlBuffer(unix.KEYCTL_READ, keyID, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("getting key size: %w", err)
	}

	// Allocate buffer and read the key data
	buf := make([]byte, size)
	_, err = unix.KeyctlBuffer(unix.KEYCTL_READ, keyID, buf, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read key from keyring: %w", err)
	}

	// Deserialize the payload
	var payload secrets.Payload
	dec := gob.NewDecoder(bytes.NewReader(buf))
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding secret payload: %w", err)
	}

	return &payload, nil
}

// Delete removes a secret from the kernel keyring by its ID.
func (k *KeyringStorage) Delete(ctx context.Context, id string) error {
	clog.FromContext(ctx).Debugf("Dispatching delete for secret %s (pid=%d, tid=%d)", id, os.Getpid(), unix.Gettid())

	respChan := make(chan keyringResponse, 1)
	workerReqChan <- keyringRequest{
		op:       "delete",
		id:       id,
		respChan: respChan,
	}

	resp := <-respChan
	return resp.err
}

// deleteOnThread performs the actual delete operation on the locked thread
func deleteOnThread(ctx context.Context, keyringId int, id string) error {
	clog.FromContext(ctx).Debugf("Deleting secret %s from keyring %d (pid=%d, tid=%d)", id, keyringId, os.Getpid(), unix.Gettid())

	// Search for the key by its ID
	keyID, err := unix.KeyctlSearch(keyringId, "user", id, 0)
	if err != nil {
		//nolint:nilerr // Key not found is not an error
		return nil
	}

	// Unlink the key from the keyring
	_, err = unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, keyringId, 0, 0)
	if err != nil {
		return fmt.Errorf("unlinking key from keyring: %w", err)
	}

	return nil
}
