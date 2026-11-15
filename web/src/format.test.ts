import { describe, expect, it } from "vitest";

import {
  change,
  count,
  country,
  daysBetween,
  money,
  size,
  tick,
} from "@app/format";

describe("numbers", () => {
  it("are in full under ten thousand and compact above", () => {
    expect(count(1284)).toBe("1,284");
    expect(count(12_900)).toBe("12.9K");
    expect(count(4_200_000)).toBe("4.2M");
  });

  it("show cents only where they matter", () => {
    expect(money(0.99)).toBe("$0.99");
    expect(money(412.4)).toBe("$412");
    expect(money(12_900)).toBe("$12.9K");
  });

  it("say a change as a share, and nothing when there was nothing before", () => {
    expect(change(112, 100)).toBeCloseTo(0.12);
    expect(size(change(97, 100)!)).toBe("3%");
    expect(change(5, 0)).toBeNull();
  });
});

describe("days", () => {
  // A day is a date, so no zone the test runs in moves it.
  it("run from one to the other, both included, across a month's end", () => {
    expect(daysBetween("2026-10-30", "2026-11-02")).toEqual([
      "2026-10-30",
      "2026-10-31",
      "2026-11-01",
      "2026-11-02",
    ]);
  });

  it("are short on an axis except where a year begins", () => {
    expect(tick("2026-11-14")).toBe("11-14");
    expect(tick("2027-01-01")).toBe("2027-01-01");
  });
});

describe("countries", () => {
  it("are named from their code, and kept as the code when unknown", () => {
    expect(country("US")).toMatch(/United States/);
    expect(country("")).toBe("Unknown");
  });
});
