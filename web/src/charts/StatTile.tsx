import { size } from "@app/format";

/**
 * A headline number: what it is, how much, and how that compares with as many days before.
 *
 * The change is colored by whether it is good, which for crashes and deletions is down, and
 * carries its arrow and sign so the color is never the only thing saying which way it went.
 */
export function StatTile({
  label,
  value,
  change,
  upIsGood = true,
  note,
}: {
  label: string;
  value: string;
  /** The change from the period before, as a share; null when there is nothing to compare. */
  change: number | null;
  upIsGood?: boolean;
  note?: string;
}) {
  const flat = change === null || Math.round(change * 100) === 0;
  const good = !flat && change! > 0 === upIsGood;
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border border-line bg-bg px-4 py-3">
      <div className="truncate text-sm text-muted">{label}</div>
      <div className="text-2xl font-semibold">{value}</div>
      <div className="flex min-h-4 flex-wrap items-baseline gap-x-2 text-xs">
        {change === null ? null : (
          <span
            className={flat ? "text-faint" : good ? "text-up" : "text-down"}
            title="Against as many days before"
          >
            {flat ? "→" : change > 0 ? "↑" : "↓"} {size(change)}
          </span>
        )}
        {note ? <span className="text-faint">{note}</span> : null}
      </div>
    </div>
  );
}
