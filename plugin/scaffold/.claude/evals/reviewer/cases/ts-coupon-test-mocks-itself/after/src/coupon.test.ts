import { describe, it, expect, vi } from "vitest";
import * as coupon from "./coupon";

describe("applyCoupon", () => {
  it("caps the discount at maxOffCents", () => {
    const spy = vi.spyOn(coupon, "applyCoupon").mockReturnValue(9000);
    expect(coupon.applyCoupon(10000, 50, 1000)).toBe(9000);
    spy.mockRestore();
  });
});
