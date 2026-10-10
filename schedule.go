package walker

import "time"

const day = 24 * time.Hour

// cycleSlot returns the latest wall clock aligned cycle boundary <= now.
// Intervals up to a day are anchored on local midnight (1h => every full hour,
// 24h => midnight), longer ones on the unix epoch. Offset shifts the boundaries.
func cycleSlot(now time.Time, interval, offset time.Duration) time.Time {
	if interval <= 0 {
		return now
	}
	var anchor time.Time
	if interval <= day {
		y, m, d := now.Date()
		anchor = time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Add(offset)
		if anchor.After(now) {
			// the offset boundary of today is still ahead - step back from yesterday's
			anchor = time.Date(y, m, d-1, 0, 0, 0, 0, now.Location()).Add(offset)
		}
	} else {
		anchor = time.Unix(0, 0).In(now.Location()).Add(offset)
	}
	elapsed := now.Sub(anchor)
	return anchor.Add(elapsed - elapsed%interval)
}
