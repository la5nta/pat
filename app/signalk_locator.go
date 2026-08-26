package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/signalk"
	"github.com/pd0mz/go-maidenhead"
)

// signalkLocatorUpdater polls Signal K every hour and updates the in-memory locator field
func (a *App) signalkLocatorUpdater(ctx context.Context) {
	// Logs first error to standard logger and the rest to the debug logger
	for logger := log.Printf; ; logger = debug.Printf {
		if err := a.updateLocatorFromSignalK(); err != nil && ctx.Err() == nil {
			logger("Failed to update locator from Signal K: %v", err)
		}
		select {
		case <-time.After(time.Hour):
			continue
		case <-ctx.Done():
			return
		}
	}
}

// updateLocatorFromSignalK connects to Signal K, gets position, and updates the config locator
func (a *App) updateLocatorFromSignalK() error {
	conn, err := signalk.DialWithToken(a.config.SignalK.Addr, a.config.SignalK.UseServerTime)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

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