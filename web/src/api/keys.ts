import type { Ask } from "@app/api/actions";

/** Every query key, in one object, so what one change makes stale is found by prefix. */
export const qk = {
  me: ["me"] as const,
  apps: ["apps"] as const,
  status: ["status"] as const,
  /** Everything read from the reports, for a finished sync to make stale at once. */
  reports: ["reports"] as const,
  sales: (ask: Ask) => ["reports", "sales", ask.days, ask.app ?? ""] as const,
  analytics: (ask: Ask) =>
    ["reports", "analytics", ask.days, ask.app ?? ""] as const,
};
