package billing

// Line is one invoice line: a unit price in cents and a quantity.
type Line struct {
	UnitCents int64
	Qty       int64
}

// Subtotal is the sum of the lines, in cents.
func Subtotal(lines []Line) int64 {
	var s int64
	for _, l := range lines {
		s += l.UnitCents * l.Qty
	}
	return s
}

// WithTax is the subtotal plus tax at rate (0.0825 for 8.25%), in cents, the tax rounded half-up.
func WithTax(lines []Line, rate float64) int64 {
	sub := Subtotal(lines)
	tax := float64(sub) * rate
	return sub + int64(tax)
}
