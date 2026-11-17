package orders

import "testing"

func TestApplyDiscountHappyPath(t *testing.T) {
	got, err := ApplyDiscount(1000, 10)
	if err != nil || got != 900 {
		t.Fatalf("got %d (%v), want 900", got, err)
	}
}
