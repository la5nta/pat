package app

import "github.com/la5nta/pat/api/types"

type WSHub interface {
	UpdateStatus()
	WriteProgress(types.Progress)
	WriteNotification(types.Notification)
	Prompt(Prompt)
	NumClients() int
	ClientAddrs() []string
}

type noopWSSocket struct{}

func (noopWSSocket) UpdateStatus()                        {}
func (noopWSSocket) WriteProgress(types.Progress)         {}
func (noopWSSocket) WriteNotification(types.Notification) {}
func (noopWSSocket) Prompt(Prompt)                        {}
func (noopWSSocket) NumClients() int                      { return 0 }
func (noopWSSocket) ClientAddrs() []string                { return []string{} }
