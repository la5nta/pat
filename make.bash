#!/usr/bin/env bash
set -e

GITREV=$(git rev-parse --short HEAD)
GO_POINT_VERSION=$(go version|awk 'match($0, /1\.[0-9]+/) { print substr( $0, RSTART+2, RLENGTH-2 )}')

set -x

go test -tags "$TAGS" ../../...

if [ "$GO_POINT_VERSION" -gt "4" ]; then
	go build -v -tags "$TAGS" -ldflags "-X \"main.GitRev=$GITREV\""
else
	go build -v -tags "$TAGS" -ldflags "-X \"main.GitRev $GITREV\""
fi