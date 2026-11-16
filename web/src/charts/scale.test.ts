import { describe, expect, it } from "vitest";

import { labelled, niceStep, ticks } from "@app/charts/scale";

const sorted = (s: Set<number>) => [...s].toSorted((a, b) => a - b);

describe("an axis", () => {
  it("steps by 1, 2 or 5 times a power of ten", () => {
    expect(niceStep(100, 4)).toBe(50);
    expect(niceStep(37, 4)).toBe(10);
    expect(niceStep(7, 4)).toBe(2);
  });

  it("runs from zero past the highest value", () => {
    expect(ticks(3, 37)).toEqual([0, 10, 20, 30, 40]);
  });

  // A day of refunds goes below the line, and the axis goes with it.
  it("runs below zero when a value does", () => {
    expect(ticks(-12, 30)).toEqual([-20, 0, 20, 40]);
  });

  it("counts in whole numbers however small the values", () => {
    expect(ticks(0, 2)).toEqual([0, 1, 2]);
    expect(ticks(0, 0)).toEqual([0, 1]);
    expect(ticks(0, 0, 4, false)).toEqual([0, 1]);
  });

  it("labels every day there is room for, always the last", () => {
    expect(sorted(labelled(7, 9))).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(sorted(labelled(7, 6))).toEqual([0, 2, 4, 6]);
    expect(sorted(labelled(30))).toEqual([4, 9, 14, 19, 24, 29]);
  });
});
