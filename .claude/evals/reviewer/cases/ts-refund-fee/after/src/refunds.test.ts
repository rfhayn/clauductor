import { describe, it, expect } from "vitest";
import { refund, RefundError } from "./refunds";

const charge = { id: "ch_1", amountCents: 10000, refundedCents: 0 };

describe("refund", () => {
  it("records a refund within 30 days in full", () => {
    expect(refund(charge, 4000, 10).refundedCents).toBe(4000);
  });
  it("refuses more than the refundable remainder", () => {
    expect(() => refund({ ...charge, refundedCents: 9000 }, 2000)).toThrow(RefundError);
  });
});
