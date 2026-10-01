export interface Charge {
  id: string;
  amountCents: number;
  refundedCents: number;
}

export class RefundError extends Error {}

/** The share of a late refund the business keeps: 5%, for refunds more than 30 days after the charge. */
export const LATE_FEE = 0.05;

/**
 * Refund part of a charge. Never refunds more than was charged and not yet refunded. A refund
 * more than 30 days after the charge keeps LATE_FEE of it: the customer receives the rest.
 */
export function refund(charge: Charge, cents: number, ageDays = 0): Charge {
  if (!Number.isInteger(cents) || cents <= 0) {
    throw new RefundError("refund must be a positive whole number of cents");
  }
  const remaining = charge.amountCents - charge.refundedCents;
  if (cents > remaining) {
    throw new RefundError(`only ${remaining} cents are refundable`);
  }
  const fee = ageDays > 30 ? Math.round(cents * LATE_FEE) : 0;
  return { ...charge, refundedCents: charge.refundedCents + cents + fee };
}
