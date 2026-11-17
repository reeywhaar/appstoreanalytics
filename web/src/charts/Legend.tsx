/**
 * Who is who in the charts below it: each app's colour as a square, whether the charts are columns
 * or lines, since a stroke as thin as a line is too little of a colour to tell one from the next,
 * and its name in text colour, never in its own.
 */
export function Legend({
  entries,
  className = "",
}: {
  entries: { id: string; name: string; color: string }[];
  className?: string;
}) {
  return (
    <ul
      className={`flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted ${className}`}
      aria-label="Apps"
    >
      {entries.map((e) => (
        <li key={e.id} className="flex items-center gap-1.5">
          <svg width={10} height={10} className="shrink-0" aria-hidden="true">
            <rect width={10} height={10} rx={2} style={{ fill: e.color }} />
          </svg>
          {e.name}
        </li>
      ))}
    </ul>
  );
}
