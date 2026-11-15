import { describe, expect, it } from "vitest";

import type { Status } from "@app/api/types";
import { pending } from "@app/islands/app/pending";

const part = { ran_at: 1, ok_at: 1, error: "" };

function status(analytics: Partial<Status["analytics"]>, apps = part): Status {
  return {
    running: false,
    requests: [],
    reading: "",
    apps: { count: 8, part: apps },
    analytics: {
      first: "",
      last: "",
      instances: 0,
      requested_apps: 0,
      requested_since: null,
      part,
      ...analytics,
    },
    sales: { on: false, first: "", last: "", days: 0, part },
    rates: { at: null, part },
  };
}

describe("why there are no analytics", () => {
  it("says what was asked for and when, while Apple has delivered nothing", () => {
    const since = Date.now() / 1000 - 3 * 3600;
    const said = pending(status({ requested_apps: 8, requested_since: since }));
    expect(said).toMatch(
      /^Asked Apple for the analytics reports of 8 apps on \d{4}-\d\d-\d\d \d\d:\d\d, 3 hours ago\. None delivered yet/,
    );
  });

  it("says why when Apple refused", () => {
    const said = pending(
      status({}, { ...part, error: "App Store Connect refused the key." }),
    );
    expect(said).toBe(
      "Apple has not been asked for analytics reports yet. App Store Connect refused the key.",
    );
  });

  it("says nothing once anything has been delivered", () => {
    expect(pending(status({ instances: 3 }))).toBeUndefined();
  });
});
