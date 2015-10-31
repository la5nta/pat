#!/usr/bin/env bash
set -e

GITREV=$(git rev-parse --short HEAD)

set -x
go test -tags "$TAGS" ../...
go build -v -tags "$TAGS" -ldflags "-X \"main.GitRev=$GITREV\""
