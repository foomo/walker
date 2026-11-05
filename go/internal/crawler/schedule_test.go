package crawler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCycleSlot(t *testing.T) {
	zurich, errLoc := time.LoadLocation("Europe/Zurich")
	assert.NoError(t, errLoc)
	at := func(loc *time.Location, d, h, m, s int) time.Time {
		return time.Date(2026, time.October, d, h, m, s, 0, loc)
	}
	for _, tc := range []struct {
		name     string
		now      time.Time
		interval time.Duration
		offset   time.Duration
		want     time.Time
	}{
		{"no interval", at(time.UTC, 10, 14, 20, 0), 0, 0, at(time.UTC, 10, 14, 20, 0)},
		{"hourly", at(time.UTC, 10, 14, 20, 0), time.Hour, 0, at(time.UTC, 10, 14, 0, 0)},
		{"hourly on boundary", at(time.UTC, 10, 14, 0, 0), time.Hour, 0, at(time.UTC, 10, 14, 0, 0)},
		{"hourly offset", at(time.UTC, 10, 14, 10, 0), time.Hour, 15 * time.Minute, at(time.UTC, 10, 13, 15, 0)},
		{"2s", at(time.UTC, 10, 14, 20, 5), 2 * time.Second, 0, at(time.UTC, 10, 14, 20, 4)},
		{"daily local midnight", at(zurich, 10, 14, 20, 0), day, 0, at(zurich, 10, 0, 0, 0)},
		{"daily offset ahead", at(zurich, 10, 2, 0, 0), day, 3 * time.Hour, at(zurich, 9, 3, 0, 0)},
		{"daily offset passed", at(zurich, 10, 4, 0, 0), day, 3 * time.Hour, at(zurich, 10, 3, 0, 0)},
		{"weekly from epoch", at(time.UTC, 10, 14, 20, 0), 7 * day, 0, at(time.UTC, 8, 0, 0, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cycleSlot(tc.now, tc.interval, tc.offset)
			assert.True(t, tc.want.Equal(got), "want %v got %v", tc.want, got)
		})
	}
}
