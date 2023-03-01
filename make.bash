#!/usr/bin/env bash
set -e

export GO111MODULE=on

if [ -d $GOOS ]; then OS=$(go env GOOS); else OS=$GOOS; fi
if [ -d $CGO_ENABLED ]; then CGO_ENABLED=$(go env CGO_ENABLED); else OS=$CGO_ENABLED; fi

GITREV=$(git rev-parse --short HEAD)
VERSION=$(grep "Version =" internal/buildinfo/VERSION.go|cut -d '"' -f2)

# Go 1.16 or later is required
GO_POINT_VERSION=$(go version| perl -ne 'm/go1\.(\d+)/; print $1;')
[ "$GO_POINT_VERSION" -lt "16" ] && echo "Go 1.16 or later required" && exit 1;

AX25VERSION="0.0.12-rc4"
AX25DIST="libax25-${AX25VERSION}"
AX25DIST_URL="http://http.debian.net/debian/pool/main/liba/libax25/libax25_${AX25VERSION}.orig.tar.gz"
function install_libax25 {
	mkdir -p .build && cd .build
	[[ -f "${AX25DIST}" ]] || curl -LSsf "${AX25DIST_URL}" | tar zx
	cd "${AX25DIST}/" && ./configure --prefix=/ && make && cd ../../
}
function build_web {
	cd web
	if [ -d $NVM_DIR ]; then
	  source $NVM_DIR/nvm.sh
	  nvm use
	fi
	npm install
	npm run production
}

[[ "$1" == "libax25" ]] && install_libax25 && exit 0;
[[ "$1" == "web" ]] && build_web && exit 0;

# Link against libax25 (statically) on Linux
if [[ "$OS" == "linux"* ]] && [[ "$CGO_ENABLED" == "1" ]]; then
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
	else
		TAGS="static $TAGS"
	fi
else
	if [[ "$OS" == "linux"* ]]; then
		echo "WARNING: CGO unavailable. libax25 (ax25+linux) will not be supported with this build."
	fi
fi

echo -e "Downloading Go dependencies..."
go mod download

echo "Running tests..."
if [[ "$SKIP_TESTS" == "1" ]]; then
	echo "Skipping."
else
	go test -tags "$TAGS" ./... github.com/la5nta/wl2k-go/...
fi
echo

echo "Building Pat v$VERSION..."
go build -tags "$TAGS" -ldflags "-X \"github.com/la5nta/pat/internal/buildinfo.GitRev=$GITREV\"" $(go list .)

# Build macOS pkg
if [[ "$OS" == "darwin"* ]] && command -v packagesbuild >/dev/null 2>&1; then
	ARCH=$(go env GOARCH)
	echo "Generating macOS installer package..."
	packagesbuild osx/pat.pkgproj
	mv 'Pat :: A Modern Winlink Client.pkg' "pat_${VERSION}_darwin_${ARCH}_unsigned.pkg"
fi

echo -e "Enjoy!"
