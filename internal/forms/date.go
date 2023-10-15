package forms

import (
	"strings"
	"time"
)

func formatDateTime(t time.Time) string    { return t.Format("2006-01-02 15:04:05") }
func formatDateTimeUTC(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05Z") }
func formatDate(t time.Time) string        { return t.Format("2006-01-02") }
func formatTime(t time.Time) string        { return t.Format("15:04:05") }
func formatDateUTC(t time.Time) string     { return t.UTC().Format("2006-01-02Z") }
func formatTimeUTC(t time.Time) string     { return t.UTC().Format("15:04:05Z") }
func formatUDTG(t time.Time) string        { return strings.ToUpper(t.UTC().Format("021504Z Jan 2006")) }
