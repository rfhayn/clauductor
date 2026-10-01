package billing

import "testing"

func TestSubtotal(t *testing.T) {
	got := Subtotal([]Line{{UnitCents: 250, Qty: 2}, {UnitCents: 99, Qty: 1}})
	if got != 599 {
		t.Fatalf("Subtotal = %d, want 599", got)
	}
}
