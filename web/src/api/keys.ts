import type { Ask } from "@app/api/actions";

/** Every query key, in one object, so what one change makes stale is found by prefix. */
export const qk = {
  me: ["me"] as const,
  apps: ["apps"] as const,
  status: ["status"] as const,
  /** A range of days, for a finished sync to make stale by prefix. */
  reports: ["report"] as const,
  report: (ask: Ask) => ["report", ask.days, ask.app ?? ""] as const,
};
