package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/wl2k-go/fbb"
)

func (a *App) promptUnconfirmedAccount() (confirmed bool) {
	accountConfirmed := func() bool {
		ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
		defer cancel()
		exists, err := cmsapi.AccountExists(ctx, a.options.MyCall)
		switch {
		case err != nil:
			// API is unavailable. Use heuristic method based on message count.
			debug.Printf("Using heuristic method. API call failed: %v", err)
			if a.Mailbox().InboxCount() != 0 || a.Mailbox().SentCount() != 0 || a.Mailbox().ArchiveCount() != 0 {
				return true
			}
		case exists:
			// API confirmed active account.
			debug.Printf("API confirmed active account")
			return true
		}
		debug.Printf("Unable to confirm active account. Prompting user...")
		resp := <-a.promptHub.Prompt(
			context.Background(),
			2*time.Minute,
			PromptKindPreAccountActivation,
			"Winlink Account activation",
		)
		return resp.Value == "confirmed"
	}
	debug.Printf("Checking for active Winlink account...")
	err := DoIfElapsed(a.options.MyCall, "account-confirmed", 100*24*time.Hour, func() error {
		if accountConfirmed() {
			return nil // Account is confirmed. Persist state with TTL.
		}
		return errors.New("account not confirmed")
	})
	debug.Printf("Account confirmation error: %v", err)
	return err == nil || err == ErrRateLimited
}

func isServiceMessage(m *fbb.Message) bool {
	return m.From().EqualString("SERVICE")
}

func isAccountActivation(from fbb.Address, subject string) bool {
	return from.EqualString("SERVICE") && strings.EqualFold(strings.TrimSpace(subject), "Your New Winlink Account")
}

var (
	reSentenceSplit = regexp.MustCompile(`[.!?]`)
	rePassword      = regexp.MustCompile("['\"`]([a-zA-Z0-9]{6,12})['\"`]")
)

func isAccountActivationMessage(m *fbb.Message) (t bool, password string) {
	if !isAccountActivation(m.From(), m.Subject()) {
		return false, ""
	}
	body, _ := m.Body()

	// Search the message for a sentence that includes the word "password" and
	// contains a quoted string of 6-12 alphanumeric characters that is not the
	// users callsign.
	sentences := reSentenceSplit.Split(body, -1)
	for _, sentence := range sentences {
		if !strings.Contains(strings.ToLower(sentence), "password") {
			continue
		}
		matches := rePassword.FindStringSubmatch(sentence)
		if len(matches) > 1 && matches[1] != m.To()[0].String() {
			return true, matches[1]
		}
	}

	return true, "" // Is activation message, but no password was identified.
}

func mockNewAccountMsg() *fbb.Message {
	m := fbb.NewMessage(fbb.Private, "SERVICE")
	m.AddTo("LA5NTA")
	m.SetSubject("Your New Winlink Account")
	m.SetBody(`A new Winlink account for 'LA5NTA' has been activated. The next time you connect to a Winlink server or gateway you will be required to use 'K1CHN7' as your account password (no quotes).

In Winlink Express you'll find the option for configuring your password under "Winlink Express Setup" in the "Files" menu. In Airmail it is called the "Radio Password" and is on the "Tools | Options | Settings" Tab. For other programs, consult the appropriate documentation or help file.

You can manage your Winlink account (to include changing your password) by logging on to the Winlink web site at https://www.winlink.org.

It is important that you establish a password recovery address as well! This address is used to send you your password if you happen to forget it. You can manage your password recovery address either at the Winlink web site or by sending an OPTIONS message to SYSTEM. See WL2K_Help category, item USER_OPTIONS for details.

Please print and save this message in case you forget your password.

Thanks for using Winlink.`)
	return m
}
