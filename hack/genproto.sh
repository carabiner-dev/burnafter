#!/bin/bash
# SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
# SPDX-License-Identifier: Apache-2.0

# Generate protobuf API code for burnafter

set -e

echo "Generating protobuf code..."

protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/burnafter.proto

echo "Done!"
