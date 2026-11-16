import { describe, expect, it } from "vitest";

import {
  change,
  count,
  counted,
  country,
  daysBetween,
  money,
  moneyAxis,
  size,
  tick,
} from "@app/format";

describe("numbers", () => {
  it("say one or many of what they count", () => {
    expect(counted(1, "download", "downloads")).toBe("1 download");
    expect(counted(0, "download", "downloads")).toBe("0 downloads");
    expect(counted(1200, "download", "downloads")).toBe("1,200 downloads");
  });

  it("are in full under ten thousand and compact above", () => {
    expect(count(1284)).toBe("1,284");
    expect(count(12_900)).toBe("12.9K");
    expect(count(4_200_000)).toBe("4.2M");
  });

  it("on an axis, show cents only on a tick that has them", () => {
    expect(moneyAxis(0)).toBe("$0");
    expect(moneyAxis(0.25)).toBe("$0.25");
    expect(moneyAxis(1500)).toBe("$1.5K");
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
    expect(daysBetween("2026-11-02", "2026-10-30")).toEqual([]);
  });

  it("run to the last day however many there are", () => {
    // A history from 2024 is more than a thousand days, and once ended in September.
    const history = daysBetween("2024-01-01", "2026-11-15");
    expect(history).toHaveLength(1050);
    expect(history[0]).toBe("2024-01-01");
    expect(history.at(-1)).toBe("2026-11-15");
    // Too many to list loses the oldest, never the latest.
    expect(daysBetween("2020-01-01", "2026-11-15", 3)).toEqual([
      "2026-11-13",
      "2026-11-14",
      "2026-11-15",
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
