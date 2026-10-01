/** A percent-off coupon, its discount capped at maxOffCents. Returns the new total in cents. */
export function applyCoupon(totalCents: number, percentOff: number, maxOffCents: number): number {
  const off = Math.min(Math.round((totalCents * percentOff) / 100), maxOffCents);
  return totalCents - off;
}
