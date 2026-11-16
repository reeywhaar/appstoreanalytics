import { useState, type ReactNode } from "react";

import { Button } from "@app/components/Button";

/** A row: what it is called, as shown and as sorted, and its figures. */
export type BarRow = {
  id: string;
  name: ReactNode;
  /** The name as text, for sorting by it; the id when left out. */
  text?: string;
  values: number[];
};

/** What the rows are sorted by: a column's index, or "name", and which way. */
type Order = { by: number | "name"; down: boolean };

/**
 * Ranked rows: a name, a thin bar for the sorted column's share of its largest, and the figures.
 * Bars on one baseline compare more exactly than slices of a whole, and one hue, because the
 * rows are ranked rather than told apart.
 *
 * Largest first by the first column until a header is pressed; pressing it again turns the
 * order round. The bar always follows the column sorted by, so the longest bar is the top
 * row's.
 */
export function BarList({
  label,
  named = "Name",
  columns,
  rows,
  limit = 8,
}: {
  label: string;
  /** What a row is: Country, App. */
  named?: string;
  columns: { label: string; format: (n: number) => string }[];
  rows: BarRow[];
  limit?: number;
}) {
  const [all, setAll] = useState(false);
  const [order, setOrder] = useState<Order>({ by: 0, down: true });
  const sorted = sort(rows, order);
  const barred = order.by === "name" ? 0 : order.by;
  const max = Math.max(0, ...rows.map((r) => r.values[barred] ?? 0));
  const shown = all ? sorted : sorted.slice(0, limit);

  /** Sorts by a column, largest first, or the other way round when it already is sorted by it. */
  const press = (by: Order["by"]) =>
    setOrder((o) =>
      o.by === by ? { by, down: !o.down } : { by, down: by !== "name" },
    );

  return (
    <div className="flex flex-col gap-2">
      <table className="w-full text-sm">
        <caption className="sr-only">{label}</caption>
        <thead>
          <tr className="text-xs text-faint">
            <Header
              label={named}
              order={order}
              by="name"
              onPress={press}
              className="text-left"
            />
            {columns.map((c, i) => (
              <Header
                key={c.label}
                label={c.label}
                order={order}
                by={i}
                onPress={press}
                className="pl-3 text-right"
              />
            ))}
          </tr>
        </thead>
        <tbody>
          {shown.map((row) => {
            const share =
              max > 0 ? Math.max(0, row.values[barred] ?? 0) / max : 0;
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

/** A column's header, which sorts by it. */
function Header({
  label,
  order,
  by,
  onPress,
  className,
}: {
  label: string;
  order: Order;
  by: Order["by"];
  onPress: (by: Order["by"]) => void;
  className: string;
}) {
  const active = order.by === by;
  return (
    <th
      className={`pb-1 font-normal ${className}`}
      aria-sort={active ? (order.down ? "descending" : "ascending") : "none"}
    >
      <button
        type="button"
        onClick={() => onPress(by)}
        className={`inline-flex items-center gap-0.5 rounded hover:text-muted ${active ? "text-muted" : ""}`}
      >
        {label}
        {active ? (
          <span aria-hidden="true">{order.down ? "↓" : "↑"}</span>
        ) : null}
      </button>
    </th>
  );
}

/** The rows in an order, ties kept as they came, which is the server's order. */
function sort(rows: BarRow[], { by, down }: Order): BarRow[] {
  const sign = down ? -1 : 1;
  return rows
    .map((row, at) => ({ row, at }))
    .toSorted((a, b) => {
      const c =
        by === "name"
          ? (a.row.text ?? a.row.id).localeCompare(b.row.text ?? b.row.id)
          : (a.row.values[by] ?? 0) - (b.row.values[by] ?? 0);
      return c === 0 ? a.at - b.at : sign * c;
    })
    .map(({ row }) => row);
}
