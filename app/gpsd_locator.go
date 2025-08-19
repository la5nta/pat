package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/gpsd"
	"github.com/pd0mz/go-maidenhead"
)

// gpsdLocatorUpdater polls GPSd every hour and updates the in-memory locator field
func (a *App) gpsdLocatorUpdater(ctx context.Context) {
	// Logs first error to standard logger and the rest to the debug logger
	for logger := log.Printf; ; logger = debug.Printf {
		if err := a.updateLocatorFromGPSd(); err != nil && ctx.Err() == nil {
			logger("Failed to update locator from GPSd: %v", err)
		}
		select {
		case <-time.After(time.Hour):
			continue
		case <-ctx.Done():
			return
		}
	}
}

// updateLocatorFromGPSd connects to GPSd, gets position, and updates the config locator
func (a *App) updateLocatorFromGPSd() error {
	conn, err := gpsd.Dial(a.config.GPSd.Addr)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()
	conn.Watch(true)

	pos, err := conn.NextPosTimeout(time.Minute)
	if err != nil {
		return fmt.Errorf("failed to provide position: %w", err)
	}

	point := maidenhead.NewPoint(pos.Lat, pos.Lon)
	locator, err := point.GridSquare()
	switch {
	case err != nil:
		return fmt.Errorf("failed to convert coordinates to locator: %w", err)
	case a.config.Locator == locator:
		return nil // Locator is up to date
	}

	log.Printf("Locator changed from %s to %s", a.config.Locator, locator)
	a.config.Locator = locator
	return nil
}
