package main

import (
	"github.com/la5nta/pat/libportapat"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

/*
This file is a launcher for the portable (libportapat) version of PAT.
It is primarily designed for being embedded into an Android's apk
package - as a library.
This package is designed to start this library on a regular PC/SBC,
for debugging or using PAT as a portable app.
*/

func main() {
	log.Println("Starting portapat...")
	libportapat.SetRootPath(getDataDirectory())
	libportapat.SetRandomizePort(false)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	err := libportapat.Start()
	if err != nil {
		log.Fatalln("Unable to start portapat:", err)
	}

	log.Println("Open your browser and navigate to: ", libportapat.GetUrlForBrowsers())
	_ = <-sigs
	libportapat.Stop()
}

func getDataDirectory() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalln("Failed to get exe path:", err)
	}
	dataDir := filepath.Join(filepath.Dir(exePath), "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		err = os.MkdirAll(dataDir, 0755)
		if err != nil {
			log.Fatalln("Failed to create data directory:", err)
		}
	}

	return dataDir
}
