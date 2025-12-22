package libportapat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/la5nta/pat/api"
	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/cli"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/libportapat/porta_paths"
	"github.com/la5nta/pat/web_frontend"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
)

func SetRootPath(rootPath string) {
	porta_paths.SetRootPath(rootPath)
}

func prepareOptions(opts *app.Options) {
	opts.MailboxPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_MAILBOX)
	opts.FormsPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_FORMS)
	opts.ConfigPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_CONFIG)
	opts.PrehooksPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_PREHOOKS)
	opts.LogPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_LOG)
	opts.EventLogPath = porta_paths.GetPortaPath(porta_paths.PATH_KIND_EVENTLOG)
}

var appRunSync sync.Mutex
var stopChan = make(chan bool)

var flagRandomisePort = false

func Start() error {
	fmt.Println("Start", runtime.GOOS)
	if !appRunSync.TryLock() {
		return errors.New("App is already running. If it is not so, try stopping the app via settings/killing the process.")
	}
	api.EmbeddedFS = web_frontend.EmbeddedFS
	if flagRandomisePort {
		eport := doRandomisePort()
		if eport != nil {
			return eport
		}
	}

	go func() {
		var opts app.Options
		prepareOptions(&opts)

		cmd, _, _, _ := cli.FindCommand([]string{"portapat", "http", ""})

		for runApp(opts, cmd, []string{}) {
			log.Println("App restarted?")
		}
	}()

	return nil
}

func Stop() {
	fmt.Println("Stop")
	stopChan <- true
	appRunSync.Unlock()
}

func runApp(opts app.Options, cmd app.Command, args []string) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app.New(opts)
	defer a.Close()

	// Graceful shutdown/reload handling.
	shouldReload := make(chan bool, 1)
	done := make(chan struct{})
	a.OnReload = func() error {
		// Avoid reloading of bad config
		if _, err := app.LoadConfig(opts.ConfigPath, cfg.DefaultConfig); err != nil {
			return fmt.Errorf("bad config: %v", err)
		}
		cancel()
		shouldReload <- true
		return nil
	}
	go func() {
		defer close(shouldReload)
		dirtyDisconnectNext := true // Do the dirty disconnect right away
		for {
			select {
			case _ = <-stopChan:
				if ok := a.AbortActiveConnection(dirtyDisconnectNext); ok {
					dirtyDisconnectNext = !dirtyDisconnectNext
					continue
				}
				cancel()
				shouldReload <- false
				return
			case <-done:
				return
			}
		}
	}()

	// Run the app
	a.Run(ctx, cmd, args)
	close(done)
	return <-shouldReload
}

// the app's version consists of the upstream version and the port's code version
// a.k.a. subverion (indicates both Java and GO changes)
const PP_SUBVERSION = "0.1"

func GetVersionStr(title string) string {
	return fmt.Sprintf("%s v%s-%s [%s]", title, buildinfo.Version, PP_SUBVERSION, runtime.GOARCH)
}

func GetUrlForBrowsers() string {
	cfgPath := porta_paths.GetPortaPath(porta_paths.PATH_KIND_CONFIG)
	cfgBinary, err := os.ReadFile(cfgPath)
	defUrl := "http://127.0.0.1:8080"

	if err != nil {
		// assuming the config does not exist
		return defUrl
	}

	var config cfg.Config
	err = json.Unmarshal(cfgBinary, &config)
	if err != nil {
		log.Println("Error parsing config file:", err)
		return defUrl
	}

	// handling NULL ipv6 addresses
	if strings.Contains(config.HTTPAddr, "[::]") {
		parts := strings.Split(config.HTTPAddr, ":")
		port := parts[len(parts)-1]
		return "http://[::1]:" + port
	}

	// handling NULL ipv4 addresses
	if strings.Contains(config.HTTPAddr, "0.0.0.0") {
		parts := strings.Split(config.HTTPAddr, ":")
		port := parts[len(parts)-1]
		return "http://127.0.0.1:" + port
	}

	return "http://" + config.HTTPAddr
}
