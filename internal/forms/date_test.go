package forms

import (
	"testing"
	"time"
)

func TestDateFormat(t *testing.T) {
	now := time.Date(2023, 12, 31, 23, 59, 59, 0, time.FixedZone("UTC-4", -4*60*60))

	tests := []struct {
		fn     func(t time.Time) string
		expect string
	}{
		{formatDateTime, "2023-12-31 23:59:59"},
		{formatDateTimeUTC, "2024-01-01 03:59:59Z"},
		{formatDate, "2023-12-31"},
		{formatTime, "23:59:59"},
		{formatDateUTC, "2024-01-01Z"},
		{formatTimeUTC, "03:59:59Z"},
		{formatUDTG, "010359Z JAN 2024"},
	}

	for i, tt := range tests {
		if got := tt.fn(now); got != tt.expect {
			t.Errorf("%d: got %q expected %q", i, got, tt.expect)
		}
	}
}
