import type { Ask } from "@app/api/actions";

/** Every query key, in one object, so what one change makes stale is found by prefix. */
export const qk = {
  me: ["me"] as const,
  apps: ["apps"] as const,
  status: ["status"] as const,
  /** A range of days, for a finished sync to make stale by prefix. */
  reports: ["report"] as const,
  report: (ask: Ask) => ["report", ask.days, ask.app ?? ""] as const,
  /** Every day of one app or all, for the charts; under report, so a sync makes it stale too. */
  history: (app: string) => ["report", "history", app] as const,
};
