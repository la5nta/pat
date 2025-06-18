package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/cmsapi"
)

const (
	MPSUsage = `subcommand [options]

subcommands:
  list [--all]    List message pickup stations for your callsign, or all MPS with --all
  clear           Delete all message pickup stations for your callsign
  add [CALLSIGN]  Add a message pickup station`

	MPSExample = `
  list            List your message pickup stations
  list --all      List all message pickup stations
  clear           Delete all your message pickup stations
  add W1AW        Add W1AW as a message pickup station`
)

func MPSHandle(ctx context.Context, a *app.App, args []string) {
	mycall := a.Options().MyCall
	if mycall == "" {
		fmt.Println("ERROR: MyCall not configured")
		os.Exit(1)
	}

	switch cmd, args := shiftArgs(args); cmd {
	case "list":
		option, _ := shiftArgs(args)
		if option == "--all" {
			err := mpsListAllHandle(ctx, mycall)

			if err != nil {
				fmt.Println("ERROR:", err)
				os.Exit(1)
			}
		} else if err := mpsListMineHandle(ctx, mycall); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	case "clear":
		if err := mpsClearHandle(ctx, a, mycall); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	case "add":
		addCall, _ := shiftArgs(args)
		if err := mpsAddHandle(ctx, a, mycall, addCall); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Missing argument, try 'mps help'.")
	}
}

func mpsListAllHandle(ctx context.Context, mycall string) error {
	mpsList, err := cmsapi.HybridStationList(ctx)

	if err != nil {
		return fmt.Errorf("failed to retrieve MPS list: %w", err)
	}

	if len(mpsList) == 0 {
		fmt.Println("No message pickup stations found.")
		return nil
	}

	fmtStr := "%-12s %-16s\n"

	// Print header
	fmt.Printf(fmtStr, "mps callsign", "forwarding type")

	// Print MPS records
	for _, station := range mpsList {
		fwdType := "unknown"
		if station.AutomaticForwarding {
			fwdType = "automatic"
		} else if station.ManualForwarding {
			fwdType = "manual"
		}

		fmt.Printf(fmtStr, station.Callsign, fwdType)
	}

	return nil
}

func mpsListMineHandle(ctx context.Context, mycall string) error {
	mpsList, err := cmsapi.MPSGet(ctx, mycall, mycall)
	if err != nil {
		return fmt.Errorf("failed to retrieve your MPS records: %w", err)
	}

	if len(mpsList) == 0 {
		fmt.Println("No message pickup stations configured for your callsign.")
		return nil
	}

	fmtStr := "%-12.12s %s\n"

	// Print header
	fmt.Printf(fmtStr, "mps callsign", "timestamp")

	// Print MPS records
	for _, mps := range mpsList {
		fmt.Printf(fmtStr, mps.MpsCallsign, mps.Timestamp.Format("2006-01-02 15:04:05"))
	}
	return nil
}

func mpsClearHandle(ctx context.Context, a *app.App, mycall string) error {
	password := getPasswordForCallsign(ctx, a, mycall)
	if password == "" {
		return fmt.Errorf("password required for clear operation")
	}

	mpsList, err := cmsapi.MPSGet(ctx, mycall, mycall)
	if err != nil {
		return fmt.Errorf("failed to retrieve your MPS records for display before clear: %w", err)
	}

	if err := cmsapi.MPSDelete(ctx, mycall, mycall, password); err != nil {
		return fmt.Errorf("failed to clear MPS records: %w", err)
	}

	fmt.Println("All message pickup stations deleted successfully.")
	fmt.Println("Previous message pickup stations:")
	for _, station := range mpsList {
		fmt.Println(station.MpsCallsign)
	}

	return nil
}

func mpsAddHandle(ctx context.Context, a *app.App, mycall, mpsCallsign string) error {
	// Validate callsign format
	mpsCallsign = strings.ToUpper(strings.TrimSpace(mpsCallsign))
	if mpsCallsign == "" {
		return fmt.Errorf("MPS callsign cannot be empty")
	}

	password := getPasswordForCallsign(ctx, a, mycall)
	if password == "" {
		return fmt.Errorf("password required for add operation")
	}

	// get list to ensure that we don't allow more than
	// 3 stations total, with 2 suggested
	mpsList, err := cmsapi.MPSGet(ctx, mycall, mycall)
	if err != nil {
		return fmt.Errorf("failed to retrieve your MPS records to check if addition is allowed: %w", err)
	}

	numMPS := len(mpsList)
	if numMPS >= 3 {
		return fmt.Errorf("configuring more than 3 message pickup stations is not allowed")
	} else if numMPS == 2 {
		fmt.Println("Warning: You already have 2 message pickup stations configured, more is not recommended. The maximum allowed is 3 stations")
	}

	if err := cmsapi.MPSAdd(ctx, mycall, mycall, password, mpsCallsign); err != nil {
		return fmt.Errorf("failed to add MPS station: %w", err)
	}

	fmt.Printf("Message pickup station %s added successfully.\n", mpsCallsign)
	return nil
}
