package run

import (
	"math"
	"time"
)

func durationSeconds(start, finish time.Time) float64 {
	seconds := finish.Sub(start).Seconds()
	if seconds < 0 {
		return 0
	}
	return seconds
}

func durationPtr(seconds float64) *float64 {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		seconds = 0
	}
	return &seconds
}

func statusDurationSeconds(status WorkStatus) float64 {
	if status.Duration == nil {
		return 0
	}
	if *status.Duration < 0 || math.IsNaN(*status.Duration) || math.IsInf(*status.Duration, 0) {
		return 0
	}
	return *status.Duration
}
