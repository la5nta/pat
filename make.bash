#!/usr/bin/env bash
set -e

export GO15VENDOREXPERIMENT=1

GITREV=$(git rev-parse --short HEAD)

# Go 1.5 or later is required
GO_POINT_VERSION=$(go version| perl -ne 'm/go1\.(\d)/; print $1;')
[ "$GO_POINT_VERSION" -lt "5" ] && echo "Go 1.5 or later required" && exit 1;

set -x

go test -tags "$TAGS" `go list ./...|grep -v vendor` `go list ./...|grep wl2k-go|egrep -v '/vendor/.*/vendor/'`
go build -v -tags "$TAGS" -ldflags "-X \"main.GitRev=$GITREV\""
