import type { ChartMode } from "@app/charts/series";

/**
 * Who is who in the charts below it: each app's colour, marked as the charts mark it, a square
 * for columns and a stroke for lines, and its name in text colour, never in its own.
 */
export function Legend({
  entries,
  mode,
}: {
  entries: { id: string; name: string; color: string }[];
  mode: ChartMode;
}) {
  return (
    <ul
      className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted"
      aria-label="Apps"
    >
      {entries.map((e) => (
        <li key={e.id} className="flex items-center gap-1.5">
          <svg width={10} height={10} className="shrink-0" aria-hidden="true">
            {mode === "bars" ? (
              <rect width={10} height={10} rx={2} style={{ fill: e.color }} />
            ) : (
              <rect
                y={4}
                width={10}
                height={2}
                rx={1}
                style={{ fill: e.color }}
              />
            )}
          </svg>
          {e.name}
        </li>
      ))}
    </ul>
  );
}
