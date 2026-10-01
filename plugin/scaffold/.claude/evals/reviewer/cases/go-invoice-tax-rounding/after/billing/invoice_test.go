package billing

import "testing"

func TestSubtotal(t *testing.T) {
	got := Subtotal([]Line{{UnitCents: 250, Qty: 2}, {UnitCents: 99, Qty: 1}})
	if got != 599 {
		t.Fatalf("Subtotal = %d, want 599", got)
	}
}

func TestWithTax(t *testing.T) {
	got := WithTax([]Line{{UnitCents: 1000, Qty: 2}}, 0.05)
	if got != 2100 {
		t.Fatalf("WithTax = %d, want 2100", got)
	}
}
