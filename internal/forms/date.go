package forms

import (
	"strings"
	"time"
)

var (
	// Now is a customizable time.Now() alias to make testing easier
	now = time.Now
	// Location defines the local timezone
	location = time.Local
)

func formatDateTime(t time.Time) string    { return t.In(location).Format("2006-01-02 15:04:05") }
func formatDateTimeUTC(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05Z07:00") }
func formatDate(t time.Time) string        { return t.In(location).Format("2006-01-02") }
func formatTime(t time.Time) string        { return t.In(location).Format("15:04:05") }
func formatDateUTC(t time.Time) string     { return t.UTC().Format("2006-01-02Z07:00") }
func formatTimeUTC(t time.Time) string     { return t.UTC().Format("15:04:05Z07:00") }
func formatUDTG(t time.Time) string        { return strings.ToUpper(t.UTC().Format("021504Z07:00 Jan 2006")) }
