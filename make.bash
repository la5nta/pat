#!/usr/bin/env bash
set -e

export GO15VENDOREXPERIMENT=1

if [ -d $GOOS ]; then OS=$(go env GOOS); else OS=$GOOS; fi

GITREV=$(git rev-parse --short HEAD)
VERSION=$(grep Version VERSION.go|cut -d '"' -f2)

# Go 1.5 or later is required
GO_POINT_VERSION=$(go version| perl -ne 'm/go1\.(\d+)/; print $1;')
[ "$GO_POINT_VERSION" -lt "5" ] && echo "Go 1.5 or later required" && exit 1;

AX25VERSION="0.0.12-rc4"
AX25DIST="libax25-${AX25VERSION}"
AX25DIST_URL="http://http.debian.net/debian/pool/main/liba/libax25/libax25_${AX25VERSION}.orig.tar.gz"
function install_libax25 {
	mkdir -p .build && cd .build
	[[ -f "${AX25DIST}" ]] || curl -LSsf "${AX25DIST_URL}" | tar zx
	cd "${AX25DIST}/" && ./configure --prefix=/ && make && cd ../../
}

[[ "$1" == "libax25" ]] && install_libax25 && exit 0;

# Link against libax25 (statically) on Linux
if [[ "$OS" == "linux"* ]]; then
	TAGS="libax25 $TAGS"
	LIB=".build/${AX25DIST}/.libs/libax25.a"
	if [[ -z "$CGO_LDFLAGS" ]] && [[ -f "$LIB" ]]; then
		export CGO_CFLAGS="-I$(pwd)/.build/${AX25DIST}"
		export CGO_LDFLAGS="$(pwd)/${LIB}"
	fi
	if [[ -z "$CGO_LDFLAGS" ]]; then
		echo "WARNING: No static libax25 library available."
		echo "  Linking against shared library instead. To fix"
		echo "  this issue, set CGO_LDFLAGS to the full path of"
		echo "  libax25.a, or run 'make.bash libax25' to download"
		echo "  and compile ${AX25DIST} in .build/"
		sleep 3;
	else
		TAGS="static $TAGS"
	fi
fi

# Update submodules (dependencies)
echo -e "Updating git submodules..."
git submodule update --init --recursive

echo -e "Regenerating web resources..."
go generate

echo "Running tests..."
go test -tags "$TAGS" `go list ./...|grep -v vendor` `go list ./...|grep wl2k-go|egrep -v '/vendor/.*/vendor/'`
echo

echo "Building Pat v$VERSION..."
go build -tags "$TAGS" -ldflags "-X \"main.GitRev=$GITREV\"" $(go list .)

# Build macOS pkg (amd64)
if [[ "$OS" == "darwin"* ]] && command -v packagesbuild >/dev/null 2>&1; then
	echo "Generating macOS installer package..."
	packagesbuild osx/pat.pkgproj
	mv 'Pat :: A Modern Winlink Client.pkg' "pat_${VERSION}_darwin_amd64_unsigned.pkg"
fi

echo -e "Enjoy!"
