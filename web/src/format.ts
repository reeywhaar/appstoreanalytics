/**
 * Numbers, money and days for a person. Days are ISO 8601 whatever the locale, and a report's
 * day is a date, never a moment: it is shown as written, in no zone.
 */

export const two = (n: number) => String(n).padStart(2, "0");

const whole = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
const compact = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

/** A count in full under ten thousand, and compact above: 1,284 / 12.9K / 4.2M. */
export function count(n: number): string {
  return Math.abs(n) < 10_000 ? whole.format(n) : compact.format(n);
}

/** A count and what it counts, one or many: 1 download, 2 downloads. */
export function counted(n: number, one: string, many: string): string {
  return `${full(n)} ${n === 1 ? one : many}`;
}

/** A count in full, for a table's column. */
export function full(n: number): string {
  return whole.format(n);
}

const dollars = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});
const cents = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});
const compactDollars = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
});

/** Dollars, with cents only where they are a noticeable part of it: $0.99, $412, $12.9K. */
export function money(n: number): string {
  if (Math.abs(n) < 100) return cents.format(n);
  if (Math.abs(n) < 10_000) return dollars.format(n);
  return compactDollars.format(n);
}

const axisDollars = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
  minimumFractionDigits: 0,
});

/** Dollars on an axis, whose ticks are round: $0, $50, $1.5K, and cents where a tick is: $0.25. */
export function moneyAxis(n: number): string {
  return Math.abs(n) < 10 && !Number.isInteger(n)
    ? cents.format(n)
    : axisDollars.format(n);
}

/** Dollars and cents, for a table's column. */
export function moneyFull(n: number): string {
  return cents.format(n);
}

/** The change from before to now, as a signed share: +12%, −3%; null when before was nothing. */
export function change(now: number, before: number): number | null {
  if (before === 0) return null;
  return (now - before) / Math.abs(before);
}

/** A change as a share, unsigned, for beside an arrow that says which way: 12%. */
export function size(share: number): string {
  return `${Math.abs(Math.round(share * 100))}%`;
}

/** A report's day as a date at noon UTC, far enough from midnight that no zone moves its day. */
export function dayOf(day: string): Date {
  return new Date(`${day}T12:00:00Z`);
}

/** The YYYY-MM-DD of a date made by dayOf. */
export function dayString(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** Every day from from to to, both included. */
export function daysBetween(from: string, to: string, most = 5000): string[] {
  // Counted back from the last day, so a span longer than most loses its oldest days and never
  // its latest: a chart opens on the last one. most only stops a mistaken span running away; the
  // server's history is three years.
  const out: string[] = [];
  const d = dayOf(to);
  if (Number.isNaN(d.getTime())) return out;
  while (out.length < most && dayString(d) >= from) {
    out.push(dayString(d));
    d.setUTCDate(d.getUTCDate() - 1);
  }
  return out.toReversed();
}

/** A day on an axis, short: 11-14, and 2026-01-01 where a year begins. */
export function tick(day: string): string {
  return day.endsWith("-01-01") ? day : day.slice(5);
}

/** A day of the week, short, in the reader's words, for the day it was in Apple's zone. */
export function weekday(day: string): string {
  return dayOf(day).toLocaleDateString(undefined, {
    weekday: "short",
    timeZone: "UTC",
  });
}

let regions: Intl.DisplayNames | undefined;

/** A country's name from its ISO code, in the reader's words, and the code when unknown. */
export function country(code: string): string {
  if (!code) return "Unknown";
  try {
    regions ??= new Intl.DisplayNames(undefined, { type: "region" });
    return regions.of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}

/** A moment, in the viewer's zone, as ISO 8601 writes a day and on a 24-hour clock: 2026-11-15 22:05. */
export function moment(unix: number): string {
  const d = new Date(unix * 1000);
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}`;
}

/** How long ago, in words: "just now", "5 minutes ago", "3 hours ago". */
export function ago(unix: number, now = new Date()): string {
  const seconds = Math.round(unix - now.getTime() / 1000);
  if (seconds > -45) return "just now";
  const words = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  for (const [unit, length] of [
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ] as const) {
    if (Math.abs(seconds) >= length) {
      return words.format(Math.round(seconds / length), unit);
    }
  }
  return words.format(seconds, "second");
}
