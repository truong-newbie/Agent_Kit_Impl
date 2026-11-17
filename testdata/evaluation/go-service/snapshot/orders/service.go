package orders

import "errors"

// ApplyDiscount calculates final order cents after applying percent (0..100).
func ApplyDiscount(totalCents int64, percent int) (int64, error) {
	if totalCents < 0 {
		return 0, errors.New("negative total")
	}
	// Intentional bug for eval baseline: fails to reject percent < 0 or percent > 100
	// and uses integer truncation instead of rounded discount deduction cleanly.
	discount := (totalCents * int64(percent)) / 100
	return totalCents - discount, nil
}
