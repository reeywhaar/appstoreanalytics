import { useState, type ReactNode } from "react";

import { Button } from "@app/components/Button";
import { Dialog } from "@app/components/Dialog";

/** Whose a part of a row's bar is, in their colour, with their own figures, column by column. */
export type BarPart = {
  id: string;
  name: string;
  color: string;
  values: number[];
};

/** A row: what it is called, as shown and as sorted, and its figures. */
export type BarRow = {
  id: string;
  name: ReactNode;
  /** The name as text, for sorting by it; the id when left out. */
  text?: string;
  values: number[];
  /** Whose the bar is, in their order: one colour each, side by side along it. */
  parts?: BarPart[];
};

/** What the rows are sorted by: a column's index, or "name", and which way. */
type Order = { by: number | "name"; down: boolean };

/**
 * Ranked rows: a name, a thin bar for the sorted column's share of its largest, and the figures.
 * Bars on one baseline compare more exactly than slices of a whole. In one colour when the rows
 * are only ranked; given whose each is, a bar is told apart by them, each one's part in their
 * colour, side by side, as a day's column is stacked.
 *
 * Largest first by the first column until a header is pressed; pressing it again turns the
 * order round. The bar always follows the column sorted by, so the longest bar is the top
 * row's.
 *
 * With detail, a row with parts opens them in a dialog: each one ranked, with their figures.
 */
export function BarList({
  label,
  named = "Name",
  columns,
  rows,
  limit = 8,
  color = "var(--color-series)",
  detail = false,
}: {
  label: string;
  /** What a row is: Country, App. */
  named?: string;
  columns: { label: string; format: (n: number) => string }[];
  rows: BarRow[];
  limit?: number;
  /** The bars' colour, where a row has no parts. */
  color?: string;
  /** Whether a row opens its parts in a dialog. */
  detail?: boolean;
}) {
  const [all, setAll] = useState(false);
  const [order, setOrder] = useState<Order>({ by: 0, down: true });
  const [opened, setOpened] = useState<BarRow | null>(null);
  const sorted = sort(rows, order);
  const barred = order.by === "name" ? 0 : order.by;
  const max = Math.max(0, ...rows.map((r) => r.values[barred] ?? 0));
  const shown = all ? sorted : sorted.slice(0, limit);
  const opens = (row: BarRow) => detail && (row.parts?.length ?? 0) > 0;

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
            const bar = (
              <Bar row={row} share={share} barred={barred} color={color} />
            );
            return (
              <tr
                key={row.id}
                className={`align-top ${opens(row) ? "cursor-pointer hover:bg-fill/60" : ""}`}
                // The whole row is a press target; the button in it is the keyboard's.
                onClick={opens(row) ? () => setOpened(row) : undefined}
              >
                <td className="w-full max-w-0 py-1.5">
                  {opens(row) ? (
                    <button
                      type="button"
                      className="block w-full rounded-sm text-left"
                      aria-label={`${row.text ?? row.id}: each app`}
                    >
                      <div className="truncate">{row.name}</div>
                      {bar}
                    </button>
                  ) : (
                    <>
                      <div className="truncate">{row.name}</div>
                      {bar}
                    </>
                  )}
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
      {detail ? (
        <Dialog
          open={opened !== null}
          onClose={() => setOpened(null)}
          title={opened?.text ?? opened?.id ?? ""}
          aside={named}
        >
          {opened ? (
            <Parts row={opened} columns={columns} label={label} />
          ) : null}
        </Dialog>
      ) : null}
    </div>
  );
}

/**
 * A row's bar: one colour, or each part in its own, side by side and a surface hairline apart,
 * the least of them a sliver rather than nothing.
 */
function Bar({
  row,
  share,
  barred,
  color,
}: {
  row: BarRow;
  share: number;
  barred: number;
  color: string;
}) {
  const parts = (row.parts ?? []).filter((p) => (p.values[barred] ?? 0) > 0);
  return (
    <div className="mt-1 h-1.5 rounded-full bg-fill">
      {share > 0 ? (
        <div
          className="flex h-1.5 gap-px overflow-hidden rounded-full"
          style={{ width: `${Math.max(1, share * 100)}%` }}
        >
          {parts.length > 0 ? (
            parts.map((p) => (
              <div
                key={p.id}
                className="h-1.5 min-w-0.5"
                style={{
                  flex: `${p.values[barred]} 1 0`,
                  backgroundColor: p.color,
                }}
              />
            ))
          ) : (
            <div className="h-1.5 flex-1" style={{ backgroundColor: color }} />
          )}
        </div>
      ) : null}
    </div>
  );
}

/** A row's parts, as a dialog shows them: its figures, then each part ranked, in its colour. */
function Parts({
  row,
  columns,
  label,
}: {
  row: BarRow;
  columns: { label: string; format: (n: number) => string }[];
  label: string;
}) {
  const parts = row.parts ?? [];
  return (
    <div className="flex flex-col gap-4 pb-2">
      <dl className="flex flex-wrap gap-x-6 gap-y-1">
        {columns.map((c, i) => (
          <div key={c.label}>
            <dt className="text-xs text-faint">{c.label}</dt>
            <dd className="text-xl font-semibold tabular-nums">
              {c.format(row.values[i] ?? 0)}
            </dd>
          </div>
        ))}
      </dl>
      <BarList
        label={`${label}: ${row.text ?? row.id}, by app`}
        named="App"
        columns={columns}
        limit={parts.length}
        rows={parts.map((p) => ({
          id: p.id,
          text: p.name,
          name: (
            <span className="flex items-center gap-1.5">
              <svg width={8} height={8} className="shrink-0" aria-hidden="true">
                <rect width={8} height={8} rx={2} style={{ fill: p.color }} />
              </svg>
              <span className="truncate">{p.name}</span>
            </span>
          ),
          values: p.values,
          parts: [p],
        }))}
      />
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
      // A header on one line: "Page views ↓" broken in two reads as two headers.
      className={`pb-1 font-normal whitespace-nowrap ${className}`}
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
