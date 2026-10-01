import { describe, it, expect } from "vitest";
import { formatCents } from "./format";

describe("formatCents", () => {
  it("groups thousands and pads the cents", () => {
    expect(formatCents(123456)).toBe("$1,234.56");
    expect(formatCents(5)).toBe("$0.05");
    expect(formatCents(0)).toBe("$0.00");
  });
  it("puts the sign before the dollar sign", () => {
    expect(formatCents(-5)).toBe("-$0.05");
  });
  it("rejects fractional and unsafe values", () => {
    expect(() => formatCents(1.5)).toThrow(RangeError);
    expect(() => formatCents(Number.MAX_SAFE_INTEGER + 2)).toThrow(RangeError);
  });
});
