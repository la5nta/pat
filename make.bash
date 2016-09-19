#!/usr/bin/env bash
set -e

export GO15VENDOREXPERIMENT=1

GITREV=$(git rev-parse --short HEAD)
VERSION=$(grep Version VERSION.go|cut -d '"' -f2)

# Go 1.5 or later is required
GO_POINT_VERSION=$(go version| perl -ne 'm/go1\.(\d)/; print $1;')
[ "$GO_POINT_VERSION" -lt "5" ] && echo "Go 1.5 or later required" && exit 1;

# Link against libax25 on Linux
if [[ "$OSTYPE" == "linux"* ]]; then
	if [[ -f "/usr/lib/libax25.a" ]]; then
		TAGS="libax25"
	else
		echo "WARNING: Not linking with libax25 - /usr/lib/libax25.a not found."
	fi
fi

# Update submodules (dependencies)
echo -e "Updating git submodules..."
git submodule update --init --recursive

echo "Running tests..."
go test -tags "$TAGS" `go list ./...|grep -v vendor` `go list ./...|grep wl2k-go|egrep -v '/vendor/.*/vendor/'`
echo

echo "Building Pat v$VERSION..."
go build -tags "$TAGS" -ldflags "-X \"main.GitRev=$GITREV\""

# Build macOS pkg (amd64)
if [[ "$OSTYPE" == "darwin"* ]] && command -v packagesbuild >/dev/null 2>&1; then
	echo "Generating macOS installer package..."
	packagesbuild osx/pat.pkgproj
	mv 'Pat :: A Modern Winlink Client.pkg' "pat_${VERSION}_darwin_amd64_unsigned.pkg"
fi

echo -e "Enjoy!"
