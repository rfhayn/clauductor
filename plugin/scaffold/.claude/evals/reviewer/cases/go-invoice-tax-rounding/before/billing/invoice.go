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
