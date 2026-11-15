import type { Status } from "@app/api/types";
import { ago, moment } from "@app/format";

/**
 * Why there are no analytics yet, as the sync last found it: not asked for, refused, or asked
 * for and not delivered. Undefined once anything has been delivered.
 */
export function pending(status: Status | undefined): string | undefined {
  if (!status || status.analytics.instances > 0) return undefined;
  const {
    requested_apps: apps,
    requested_since: since,
    part,
  } = status.analytics;
  if (apps > 0 && since !== null) {
    const which = apps === 1 ? "1 app" : `${apps} apps`;
    return `Asked Apple for the analytics reports of ${which} on ${moment(since)}, ${ago(since)}. None delivered yet: Apple's first usually comes within one to two days, with history back to January 2024, and shows here on the next hourly sync.`;
  }
  const refusal = status.apps.part.error || part.error;
  if (refusal) {
    return `Apple has not been asked for analytics reports yet. ${refusal}`;
  }
  return "The first sync is asking Apple for each app's analytics reports.";
}
