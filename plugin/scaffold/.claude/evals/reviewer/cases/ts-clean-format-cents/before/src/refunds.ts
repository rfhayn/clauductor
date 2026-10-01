export interface Charge {
  id: string;
  amountCents: number;
  refundedCents: number;
}

export class RefundError extends Error {}

/** Refund part of a charge. Never refunds more than was charged and not yet refunded. */
export function refund(charge: Charge, cents: number): Charge {
  if (!Number.isInteger(cents) || cents <= 0) {
    throw new RefundError("refund must be a positive whole number of cents");
  }
  const remaining = charge.amountCents - charge.refundedCents;
  if (cents > remaining) {
    throw new RefundError(`only ${remaining} cents are refundable`);
  }
  return { ...charge, refundedCents: charge.refundedCents + cents };
}
