// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Command gzip compresses a file to <file>.gz using Go's compress/gzip and then
// removes the original, mimicking `gzip -f`. Unlike the system gzip binary
// (whose DEFLATE output differs between implementations, e.g. BSD/Apple vs GNU),
// this produces byte-identical output on any host for a given Go version, so the
// embedded server binaries are reproducible across developer machines and CI.
package main

import (
	"compress/gzip"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gzip <file>")
		os.Exit(2)
	}
	if err := compress(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "gzip: %v\n", err)
		os.Exit(1)
	}
}

func compress(path string) (err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	out, err := os.Create(path + ".gz")
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	gw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	// Pin every header field so the output depends only on the input bytes and
	// the Go version: no filename, no comment, zero mtime, OS=255 (unknown).
	gw.Name = ""
	gw.Comment = ""
	gw.ModTime = time.Time{}
	gw.OS = 255

	if _, werr := gw.Write(data); werr != nil {
		return werr
	}
	if cerr := gw.Close(); cerr != nil {
		return cerr
	}

	// Replace the original, like `gzip -f`.
	return os.Remove(path)
}
