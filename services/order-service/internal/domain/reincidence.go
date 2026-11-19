package domain

import (
	"sort"
	"time"
)

const (
	// ReincidenceThreshold is the number of customer incidents inside one window that marks a
	// recidivist (ADR 0006, section 1: "una tercera dentro de ese plazo").
	ReincidenceThreshold = 3
	// ReincidenceWindowDays is the length of the window the first incident opens.
	ReincidenceWindowDays = 14
)

// ReincidenceReached reports whether the incident times contain a window with the threshold. The
// first incident opens a window of 14 days (the last day included); the first incident after it
// opens the next one. The window does not slide: 0, 10, 16 and 24 days is not reincidence.
// The order of the input does not matter.
func ReincidenceReached(times []time.Time) bool {
	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })

	for start := 0; start < len(sorted); {
		deadline := sorted[start].AddDate(0, 0, ReincidenceWindowDays)
		end := start
		for end < len(sorted) && !sorted[end].After(deadline) {
			end++
		}
		if end-start >= ReincidenceThreshold {
			return true
		}
		start = end
	}
	return false
}
