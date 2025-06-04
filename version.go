package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/patapi"
	"github.com/spf13/pflag"
)

func versionHandle(ctx context.Context, args []string) {
	var check bool
	set := pflag.NewFlagSet("version", pflag.ExitOnError)
	set.BoolVarP(&check, "check", "c", false, "Check if new version is available")
	set.Parse(args)

	fmt.Printf("%s %s\n", buildinfo.AppName, buildinfo.VersionString())
	if !check {
		return
	}

	fmt.Println()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	release, err := patapi.GetLatestVersion(ctx)
	if err != nil {
		log.Printf("Error checking version: %v", err)
		return
	}

	current := buildinfo.Version
	fmt.Printf("Current version: %s\n", current)
	fmt.Printf("Latest version:  %s\n", release.Version)

	// Compare using version parser
	currentVer, err := version.NewVersion(current)
	if err != nil {
		log.Printf("Warning: Invalid version format (current: %s): %v", current, err)
		return
	}
	latestVer, err := version.NewVersion(release.Version)
	if err != nil {
		log.Printf("Warning: Invalid version format (latest: %s): %v", release.Version, err)
		return
	}

	switch currentVer.Compare(latestVer) {
	case 0:
		fmt.Println("You are running the latest version!")
	case -1:
		fmt.Printf("A new version is available!\nRelease URL: %s\n", release.ReleaseURL)
	case 1:
		fmt.Println("You are running a development version!")
	}
}
