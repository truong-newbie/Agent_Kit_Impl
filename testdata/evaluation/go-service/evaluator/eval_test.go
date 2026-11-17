package orders

import "testing"

func TestPrivateEvaluatorApplyDiscount(t *testing.T) {
	if _, err := ApplyDiscount(1000, -5); err == nil {
		t.Fatal("expected error for negative discount percent")
	}
	if _, err := ApplyDiscount(1000, 101); err == nil {
		t.Fatal("expected error for discount percent > 100")
	}
	got, err := ApplyDiscount(1000, 100)
	if err != nil || got != 0 {
		t.Fatalf("expected 0 for 100%% discount, got %d (%v)", got, err)
	}
}
