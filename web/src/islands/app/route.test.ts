import { describe, expect, it } from "vitest";

import { parse, paths } from "@app/islands/app/route";

describe("an address", () => {
  it("is every app for the last 30 days at the root", () => {
    expect(parse("/")).toEqual({ page: "dashboard", app: null, days: 30 });
  });

  it("names one app by its Apple ID, and a range by its days", () => {
    expect(parse("/apps/1234567890?days=90")).toEqual({
      page: "dashboard",
      app: "1234567890",
      days: 90,
    });
  });

  // A hand-edited range is the default rather than a page that cannot be drawn.
  it("takes a range it does not offer as the default", () => {
    expect(parse("/?days=13")).toEqual({
      page: "dashboard",
      app: null,
      days: 30,
    });
  });

  it("is missing when it names nothing", () => {
    expect(parse("/apps/dots")).toEqual({ page: "missing" });
    expect(parse("/settings/relay")).toEqual({ page: "missing" });
    expect(parse("/nope")).toEqual({ page: "missing" });
  });

  it("is the settings page at /settings", () => {
    expect(parse("/settings")).toEqual({ page: "settings" });
    expect(parse("/status")).toEqual({ page: "status" });
  });

  it("is made back from what it names", () => {
    for (const address of ["/", "/apps/42?days=7", "/?days=365"]) {
      const route = parse(address);
      if (route.page !== "dashboard") throw new Error(address);
      expect(paths.dashboard(route.app, route.days)).toBe(address);
    }
  });
});
