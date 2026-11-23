package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
)

func (a *App) rmsListPath() string {
	name := "rmslist"
	if len(a.config.ServiceCodes) != 1 || a.config.ServiceCodes[0] != "PUBLIC" {
		name += "-" + strings.Join(a.config.ServiceCodes, "-")
	}
	return filepath.Join(directories.DataDir(), name+".json")
}

// RequestRMSListViaRF posts a CMS Inquiry request (the same mechanism as Winlink Express'
// "Update Via Radio") to the outbox. The reply arrives in a later session and is applied
// by applyRMSListReply.
//
// mode is one of the Mode values accepted by the gateway status API (e.g. vara, ardop, pactor,
// packet, robustpacket, allhf). It bounds the size of the reply, which matters over RF.
func (a *App) RequestRMSListViaRF(mode string) (mid string, err error) {
	if mode == "" {
		return "", fmt.Errorf("mode is required (e.g. vara, ardop, pactor, packet, robustpacket, allhf)")
	}
	params := url.Values{"Mode": {mode}, "key": {cmsapi.AccessKey}}
	for _, c := range a.config.ServiceCodes {
		params.Add("ServiceCodes", c)
	}
	msg := fbb.NewMessage(fbb.Private, a.options.MyCall)
	msg.AddTo("INQUIRY")
	msg.SetSubject("CMSWEBSVC")
	msg.SetBody(cmsapi.PathGatewayStatus + "?" + params.Encode())
	if err := a.Mailbox().AddOut(msg); err != nil {
		return "", err
	}
	return msg.MID(), nil
}

// applyRMSListReply stores an Inquiry reply as the RMS list cache. It returns true if msg was such a reply.
func (a *App) applyRMSListReply(msg *fbb.Message) bool {
	if !strings.HasPrefix(msg.Subject(), "INQUIRY: ") {
		return false
	}
	body, _ := msg.Body()
	var status cmsapi.GatewayStatus
	if err := json.Unmarshal([]byte(body), &status); err != nil || len(status.Gateways) == 0 {
		return false // Not an RMS list. Let it be handled as an ordinary service message.
	}
	// Atomic replace, so a crash can't leave a truncated cache.
	path := a.rmsListPath()
	tmp := path + ".tmp"
	err := os.WriteFile(tmp, bytes.TrimSpace([]byte(body)), 0o644)
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		log.Printf("Failed to store RMS list received via radio: %v", err)
		return false
	}
	log.Printf("RMS list updated via radio (%d gateways)", len(status.Gateways))
	// File the reply away, as it has served its purpose. Failure only leaves it in the inbox.
	box := a.Mailbox().MBoxPath
	src := filepath.Join(box, mailbox.DIR_INBOX, msg.MID()+mailbox.Ext)
	if err := os.Rename(src, filepath.Join(box, mailbox.DIR_ARCHIVE, msg.MID()+mailbox.Ext)); err != nil {
		debug.Printf("Could not archive RMS list reply: %v", err)
	}
	return true
}
