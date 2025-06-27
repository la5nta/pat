package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/debug"

	"github.com/howeyc/gopass"
	"github.com/pd0mz/go-maidenhead"
)

func InitHandle(ctx context.Context, a *app.App, args []string) {
	// Exit on context cancellation (os signal etc)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			fmt.Println()
			os.Exit(1)
		case <-done:
		}
	}()

	cfg, err := app.LoadConfig(a.Options().ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Pat Initial Configuration")
	fmt.Println("=========================")
	fmt.Print("(Press ctrl+c at any time to abort)\n\n")

	// Prompt for callsign
	callsign := prompt("Enter your callsign", cfg.MyCall)
	if callsign == "" {
		log.Fatal("Callsign is required")
	}
	cfg.MyCall = strings.ToUpper(callsign)

	// Prompt for Maidenhead grid square
	locator := prompt("Enter your Maidenhead locator", cfg.Locator)
	if locator == "" {
		log.Fatal("Maidenhead locator is required")
	}
	if _, err := maidenhead.ParseLocator(locator); err != nil {
		fmt.Printf("⚠ %q might be an invalid locator. Using it anyway.\n", locator)
	}
	cfg.Locator = locator

	// Check if account exists via Winlink API
	fmt.Printf("\nChecking Winlink account: %s...\n", callsign)
	switch exists, err := accountExists(ctx, callsign); {
	case err != nil:
		fmt.Println("⚠ Check failed due to network error. Assuming account exists.")
		handleExistingAccount(ctx, &cfg)
	case exists:
		fmt.Println("✓ Account exists")
		handleExistingAccount(ctx, &cfg)
	case !exists:
		fmt.Println("✗ Account does not exist")
		handleNewAccount(ctx, &cfg)
	}

	// Write the new/modified config
	if err := app.WriteConfig(cfg, a.Options().ConfigPath); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nThat's it! Basic configuration is set. For advanced settings, run '%s configure' or use the web gui.\n", os.Args[0])
}

// promptPassword prompts the user to enter a password twice for confirmation
func promptPassword() string {
	for {
		fmt.Println("\nPlease choose a password for your account (6-12 characters)")
		password1, err := gopass.GetPasswdPrompt("Enter password: ", true, os.Stdin, os.Stdout)
		if err != nil {
			log.Fatal(err)
		}
		if len(password1) < 6 || len(password1) > 12 {
			fmt.Println("✗ Password can be no less than 6 and no more than 12 characters long")
			continue
		}

		password2, err := gopass.GetPasswdPrompt("Confirm password: ", true, os.Stdin, os.Stdout)
		if err != nil {
			log.Fatal(err)
		}
		if string(password1) != string(password2) {
			fmt.Println("✗ Passwords do not match. Please try again.")
			continue
		}

		return string(password1)
	}
}

// handleNewAccount guides the user through creating a new Winlink account
func handleNewAccount(ctx context.Context, cfg *cfg.Config) {
	// This function is designed with **GDPR compliance** in mind, specifically addressing the roles
	// and responsibilities of a desktop application developer when handling user credentials
	// for a third-party service (Winlink).
	//
	// --- GDPR Compliance Summary ---
	//
	// 1.  **Role as Data Controller:**
	// For the process of collecting, transferring to Winlink, and securely storing user credentials (callsign, password, recovery email) locally, this application acts as a **Data Controller**. We determine the purpose and means of this specific data processing.
	//
	// 2.  **Lawful Basis for Processing (Consent):**
	// User **consent** (GDPR Article 6(1)(a)) is the chosen legal basis.
	// -   **Informed Consent:** The user is provided with a clear, concise, and prominent consent dialogue that explicitly states:
	// -   Which data (callsign, password, recovery email) is collected.
	// -   The purpose of collection (Winlink account creation).
	// -   That data is sent *directly* to Winlink's API.
	// -   That callsign and password will be stored *locally* in the application's configuration file for continued access, and that the developer does not have access to these local credentials.
	// -   Links to Winlink's Terms and Conditions and Privacy Policy.
	// -   **Unambiguous Consent:** Consent is obtained through an active, explicit action by the user (repeating their callsign to confirm). This goes beyond a simple "Yes/No" to demonstrate clear intent.
	//
	// 3.  **Transparency and Information Duty (GDPR Article 13):**
	// The consent dialogue fulfills the information requirements by clearly communicating:
	// -   The identity of the controller (the application/developer implicitly).
	// -   The purposes of processing.
	// -   The recipients of the data (Winlink).
	// -   The fact of local storage and its purpose.
	// -   Links to relevant third-party policies (Winlink).
	// -   Implicitly, users retain rights over their data, both with Winlink and for the locally stored credentials (e.g., through application features for credential management).
	//
	// 4.  **Security and Data Protection by Design (GDPR Articles 25 & 32):**
	// -   **Data Minimization:** Only necessary data for account creation and local access is collected.
	// -   **Secure Transmission:** All communication with Winlink's API (including credentials) occurs over **HTTPS/TLS** to ensure data integrity and confidentiality during transit.
	//
	// By adhering to these principles, this function aims to ensure robust GDPR compliance for the account creation and local credential storage process.

	fmt.Println("\nWould you like to create a new Winlink account? It is highly recommended to do so.")
	resp := prompt("Create account?", "Y", "n")
	if resp != "Y" && strings.ToLower(resp) != "y" && strings.ToLower(resp) != "yes" {
		fmt.Println("\n⚠ Account creation skipped. If you connect to the Winlink system without an active account, an over-the-air activation process will be initiated by the CMS. You'll receive a generated password the first time you connect. DO NOT LOSE THIS PASSWORD, AS YOU WILL BE LOCKED OUT OF THE SYSTEM.")
		if resp := prompt("Continue without an account?", "Y", "n"); strings.ToLower(resp) == "y" || strings.ToLower(resp) == "yes" {
			return
		}
	}

	// Prompt for password
	password := promptPassword()

	// Prompt for recovery email
	fmt.Println("\nWould you like to set a password recovery email? This is optional, but highly recommended.")
	recoveryEmail := prompt("Password recovery email (optional)", "")
	if recoveryEmail == "" {
		fmt.Println("⚠ Warning: You have chosen not to provide a password recovery email. If you proceed and forget your password, it cannot be recovered!")
		resp := prompt("Are you sure?", "N", "y")
		if resp != "y" && strings.ToLower(resp) != "yes" {
			log.Fatal("Winlink account creation cancelled")
		}
	}

	getConsent := func(callsign string) bool {
		fmt.Println()
		fmt.Println("======== CONSENT REQUIRED ========")
		fmt.Println("To create your Winlink account, we'll send your chosen callsign, password, and recovery email address directly to the Winlink system.")
		fmt.Println()
		fmt.Println("Your callsign and password will also be stored locally on your computer in the configuration file. This is so you can log in and use Winlink services directly from here.")
		fmt.Println()
		fmt.Println("By proceeding, you agree that your data will be handled according to Winlink's Terms, Conditions and Privacy Policy:")
		fmt.Println("* https://winlink.org/terms_conditions (Terms, Conditions and Privacy Policy)")
		fmt.Println("")
		fmt.Println("Do you agree to create your Winlink account and store your credentials locally?")
		fmt.Println("==================================")
		for {
			switch resp := strings.ToUpper(prompt("Repeat your callsign to confirm your consent", "")); resp {
			case "":
				return false
			case callsign:
				return true
			default:
				fmt.Println("✗ Callsigns do not match. Please try again.")
			}
		}
	}
	if consent := getConsent(cfg.MyCall); !consent {
		log.Fatal("Winlink account creation cancelled")
	}
	fmt.Println("✓ Consent granted")

	// Create the account
	fmt.Println("\nCreating Winlink account...")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := cmsapi.AccountAdd(ctx, cfg.MyCall, password, recoveryEmail)
	if err != nil {
		log.Fatalf("Failed to create Winlink account: %v", err)
	}

	fmt.Printf("✓ Congratulations! Your Winlink account for %s has been successfully created.\n", cfg.MyCall)
	cfg.SecureLoginPassword = password
}

func handleExistingAccount(ctx context.Context, cfg *cfg.Config) {
	// Prompt for password and validate
	fmt.Println()
L:
	for {
		promptStr := "Enter account password: "
		if cfg.SecureLoginPassword != "" {
			promptStr = promptStr[:len(promptStr)-2] + fmt.Sprintf(" [%s]: ", strings.Repeat("*", len(cfg.SecureLoginPassword)))
		}
		password, err := gopass.GetPasswdPrompt(promptStr, true, os.Stdin, os.Stdout)
		switch {
		case err != nil:
			log.Fatal(err)
		case len(password) == 0:
			if cfg.SecureLoginPassword != "" {
				break // Use whatever exists now.
			}
			// TODO: What about users that use Pat for P2P exclusively?
			fmt.Println("✗ Account password is required")
			continue L // Prompt again
		default:
			cfg.SecureLoginPassword = string(password)
		}

		fmt.Println("Checking password...")
		switch valid, err := validatePassword(ctx, cfg.MyCall, cfg.SecureLoginPassword); {
		case err != nil:
			fmt.Println("⚠ Password verification failed. Assuming password is correct.")
			break L
		case valid:
			fmt.Println("✓ Password verified")
			break L
		case !valid:
			fmt.Println("✗ Invalid password")
		}
	}

	// Verify password recovery email is set
	fmt.Println("\nChecking for password recovery email...")
	switch exists, err := getPasswordRecoveryEmail(context.Background(), cfg.MyCall, cfg.SecureLoginPassword); {
	case err != nil:
		fmt.Println("⚠ Password recovery email check failed. Assuming it is set.")
	case exists == "":
		fmt.Printf("✗ No password recovery email set\n")
		handleMissingPasswordRecoveryEmail(context.Background(), *cfg)
	default:
		fmt.Printf("✓ Password recovery email: %s\n", exists)
	}
}

func handleMissingPasswordRecoveryEmail(ctx context.Context, cfg cfg.Config) {
	fmt.Println()
	fmt.Println("Would you like to set a password recovery email now? This is highly recommended.")
	email := prompt("Enter password recovery email (optional)", "")
	if email == "" {
		fmt.Println("No email provided, continuing without setting password recovery email")
		return
	}

	fmt.Println("Setting password recovery email...")
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := cmsapi.PasswordRecoveryEmailSet(ctx, cfg.MyCall, cfg.SecureLoginPassword, email); err != nil {
		fmt.Printf("⚠ Failed to set password recovery email: %v\n", err)
		return
	}

	fmt.Printf("✓ Password recovery email set to: %s\n", email)
}

func accountExists(ctx context.Context, callsign string) (exists bool, err error) {
	for retry := 0; retry < 5; retry++ {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		exists, err = cmsapi.AccountExists(ctx, callsign)
		if err == nil {
			break
		}
		debug.Printf("Winlink API call failed: %v. Retrying...", err)
		time.Sleep(time.Second)
	}
	return exists, err
}

func validatePassword(ctx context.Context, callsign, password string) (valid bool, err error) {
	for retry := 0; retry < 5; retry++ {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		valid, err = cmsapi.ValidatePassword(ctx, callsign, password)
		cancel()
		if err == nil {
			break
		}
		debug.Printf("Winlink API call failed: %v. Retrying...", err)
		time.Sleep(time.Second)
	}
	return valid, err
}

func getPasswordRecoveryEmail(ctx context.Context, callsign, password string) (email string, err error) {
	for retry := 0; retry < 5; retry++ {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		email, err = cmsapi.PasswordRecoveryEmailGet(ctx, callsign, password)
		cancel()
		if err == nil {
			break
		}
		debug.Printf("Winlink API call failed: %v. Retrying...", err)
	}
	return email, err
}
