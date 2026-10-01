package billing

// Line is one invoice line: a unit price in cents and a quantity.
type Line struct {
	UnitCents int64
	Qty       int64
}

// Total is the line's price times its quantity, in cents.
func (l Line) Total() int64 {
	return l.UnitCents * l.Qty
}

// Subtotal is the sum of the lines, in cents.
func Subtotal(lines []Line) int64 {
	var s int64
	for _, l := range lines {
		s += l.Total()
	}
	return s
}
