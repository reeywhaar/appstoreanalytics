import { useState, type ReactNode } from "react";

import { Button } from "@app/components/Button";

export type BarRow = { id: string; name: ReactNode; values: number[] };

/**
 * Ranked rows, largest first: a name, a thin bar for the first column's share of the largest,
 * and the figures. Bars on one baseline compare more exactly than slices of a whole, and one
 * hue, because the rows are ranked rather than told apart.
 */
export function BarList({
  label,
  columns,
  rows,
  limit = 8,
}: {
  label: string;
  columns: { label: string; format: (n: number) => string }[];
  rows: BarRow[];
  limit?: number;
}) {
  const [all, setAll] = useState(false);
  const max = Math.max(0, ...rows.map((r) => r.values[0] ?? 0));
  const shown = all ? rows : rows.slice(0, limit);
  return (
    <div className="flex flex-col gap-2">
      <table className="w-full text-sm">
        <caption className="sr-only">{label}</caption>
        <thead>
          <tr className="text-xs text-faint">
            <th className="pb-1 text-left font-normal">
              <span className="sr-only">Name</span>
            </th>
            {columns.map((c) => (
              <th key={c.label} className="pb-1 pl-3 text-right font-normal">
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {shown.map((row) => {
            const share = max > 0 ? Math.max(0, row.values[0] ?? 0) / max : 0;
            return (
              <tr key={row.id} className="align-top">
                <td className="w-full max-w-0 py-1.5">
                  <div className="truncate">{row.name}</div>
                  <div className="mt-1 h-1.5 rounded-full bg-fill">
                    {share > 0 ? (
                      <div
                        className="h-1.5 rounded-full bg-series"
                        style={{ width: `${Math.max(1, share * 100)}%` }}
                      />
                    ) : null}
                  </div>
                </td>
                {row.values.map((v, i) => (
                  <td
                    key={columns[i]?.label ?? i}
                    className="py-1.5 pl-3 text-right whitespace-nowrap tabular-nums"
                  >
                    {columns[i]?.format(v)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
      {rows.length > limit ? (
        <Button
          variant="link"
          className="self-start"
          onClick={() => setAll(!all)}
        >
          {all ? "Show fewer" : `Show all ${rows.length}`}
        </Button>
      ) : null}
    </div>
  );
}
