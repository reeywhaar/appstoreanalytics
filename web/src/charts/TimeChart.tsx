import {
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react";

import { labelled, ticks } from "@app/charts/scale";
import { tick, weekday } from "@app/format";

/**
 * One measure over a range of days: columns for a month or so, where each day is wide enough
 * to be a mark of its own, and a line with a wash under it past that.
 *
 * A day with no value is not known yet, which is not zero: it is drawn as nothing, and a line
 * breaks across it rather than dropping to the floor. One series, so no legend: the card's
 * title names it. Every value is also in the table under it, so the hover is never the only way
 * to read one.
 */
export function TimeChart({
  label,
  days,
  values,
  format,
  axis,
  whole = true,
  height = 168,
}: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  /** A value as the tooltip and the table say it. */
  format: (n: number) => string;
  /** A value as the axis says it, shorter. */
  axis: (n: number) => string;
  whole?: boolean;
  height?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const width = useWidth(box);
  const [at, setAt] = useState<number | null>(null);

  const n = days.length;
  const known = values.filter((v): v is number => v !== undefined);
  const yTicks = ticks(Math.min(0, ...known), Math.max(0, ...known), 4, whole);
  const lo = yTicks[0]!;
  const hi = yTicks[yTicks.length - 1]!;
  const left = Math.max(...yTicks.map((t) => axis(t).length)) * 6.5 + 12;
  const right = 18;
  const top = 8;
  const bottom = 22;
  const plotW = Math.max(10, width - left - right);
  const plotH = height - top - bottom;
  const band = plotW / Math.max(1, n);
  const x = (i: number) => left + band * i + band / 2;
  const y = (v: number) => top + plotH - ((v - lo) / (hi - lo || 1)) * plotH;
  const base = y(0);
  const columns = n <= 92;
  const xLabels = labelled(n, Math.max(2, Math.floor(plotW / 64)));

  const pick = (e: PointerEvent<SVGRectElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const i = Math.floor((e.clientX - rect.left) / band);
    setAt(Math.min(n - 1, Math.max(0, i)));
  };

  const step = (e: KeyboardEvent<HTMLDivElement>) => {
    const moves: Record<string, number> = {
      ArrowLeft: -1,
      ArrowRight: 1,
      Home: -n,
      End: n,
    };
    const move = moves[e.key];
    if (move === undefined) return;
    e.preventDefault();
    setAt((i) => Math.min(n - 1, Math.max(0, (i ?? n - 1) + move)));
  };

  const said =
    at === null
      ? null
      : {
          day: days[at]!,
          value: values[at],
          left: Math.min(Math.max(0, x(at) - 70), Math.max(0, width - 140)),
        };

  return (
    <div className="flex flex-col gap-1">
      <div
        ref={box}
        className="relative rounded-md outline-offset-2 focus-visible:outline-2 focus-visible:outline-brand"
        tabIndex={0}
        role="group"
        aria-label={`${label}. Arrow keys move from day to day.`}
        onKeyDown={step}
        onFocus={() => setAt((i) => i ?? n - 1)}
        onBlur={() => setAt(null)}
      >
        <svg
          width={width}
          height={height}
          className="block overflow-visible"
          aria-hidden="true"
        >
          {yTicks.map((t) => (
            <g key={t}>
              <line
                x1={left}
                x2={left + plotW}
                y1={y(t)}
                y2={y(t)}
                className={t === 0 ? "stroke-axis" : "stroke-grid"}
                strokeWidth={1}
                shapeRendering="crispEdges"
              />
              <text
                x={left - 8}
                y={y(t)}
                dy="0.32em"
                textAnchor="end"
                className="fill-faint text-[11px] tabular-nums"
              >
                {axis(t)}
              </text>
            </g>
          ))}
          {days.map((day, i) =>
            xLabels.has(i) ? (
              <text
                key={day}
                x={x(i)}
                y={top + plotH + 16}
                textAnchor="middle"
                className="fill-faint text-[11px] tabular-nums"
              >
                {tick(day)}
              </text>
            ) : null,
          )}
          {columns ? (
            values.map((v, i) =>
              v === undefined || v === 0 ? null : (
                <path
                  key={days[i]}
                  d={column(
                    x(i),
                    Math.min(24, Math.max(1, band - 2)),
                    base,
                    y(v),
                  )}
                  className="fill-series transition-opacity"
                  opacity={at === null || at === i ? 1 : 0.45}
                />
              ),
            )
          ) : (
            <Line values={values} x={x} y={y} base={base} />
          )}
          {at !== null && !columns ? (
            <line
              x1={x(at)}
              x2={x(at)}
              y1={top}
              y2={top + plotH}
              className="stroke-axis"
              strokeWidth={1}
              shapeRendering="crispEdges"
            />
          ) : null}
          {at !== null && !columns && values[at] !== undefined ? (
            <circle
              cx={x(at)}
              cy={y(values[at]!)}
              r={4}
              className="fill-series stroke-bg"
              strokeWidth={2}
            />
          ) : null}
          <rect
            x={left}
            y={top}
            width={plotW}
            height={plotH}
            fill="transparent"
            onPointerMove={pick}
            onPointerDown={pick}
            onPointerLeave={() => setAt(null)}
          />
        </svg>
        {said ? (
          <div
            className="pointer-events-none absolute top-0 w-[140px] rounded-md border border-line bg-bg px-2.5 py-1.5 text-xs shadow-sm"
            style={{ left: said.left }}
          >
            <div className="text-sm font-semibold text-fg tabular-nums">
              {said.value === undefined
                ? "Not reported yet"
                : format(said.value)}
            </div>
            <div className="text-muted">
              {weekday(said.day)} {said.day}
            </div>
          </div>
        ) : null}
        <p className="sr-only" aria-live="polite">
          {said
            ? `${said.day}: ${said.value === undefined ? "not reported yet" : format(said.value)}`
            : ""}
        </p>
      </div>
      <Table label={label} days={days} values={values} format={format} />
    </div>
  );
}

/** The line, broken where a day is not known, with a wash under each piece. */
function Line({
  values,
  x,
  y,
  base,
}: {
  values: (number | undefined)[];
  x: (i: number) => number;
  y: (v: number) => number;
  base: number;
}) {
  const runs: [number, number][][] = [];
  let run: [number, number][] = [];
  values.forEach((v, i) => {
    if (v === undefined) {
      if (run.length) runs.push(run);
      run = [];
    } else {
      run.push([x(i), y(v)]);
    }
  });
  if (run.length) runs.push(run);
  return (
    <>
      {runs.map((points) => {
        const [first] = points;
        const last = points[points.length - 1]!;
        const line = points
          .map(([px, py], i) => `${i ? "L" : "M"}${px},${py}`)
          .join("");
        if (points.length === 1) {
          return (
            <circle
              key={first![0]}
              cx={first![0]}
              cy={first![1]}
              r={2}
              className="fill-series"
            />
          );
        }
        return (
          <g key={first![0]}>
            <path
              d={`${line}L${last[0]},${base}L${first![0]},${base}Z`}
              className="fill-series-wash"
            />
            <path
              d={line}
              fill="none"
              className="stroke-series"
              strokeWidth={2}
              strokeLinejoin="round"
              strokeLinecap="round"
            />
          </g>
        );
      })}
    </>
  );
}

/** A column from the baseline to its value: square at the baseline, rounded at the value. */
function column(cx: number, w: number, base: number, end: number): string {
  const h = Math.abs(end - base);
  if (h < 0.5) return "";
  const r = Math.min(4, w / 2, h);
  const l = cx - w / 2;
  const rr = cx + w / 2;
  const dir = end < base ? 1 : -1;
  return `M${l},${base}V${end + dir * r}Q${l},${end} ${l + r},${end}H${rr - r}Q${rr},${end} ${rr},${end + dir * r}V${base}Z`;
}

/** The same values as a table, for whoever cannot or would rather not read them off a chart. */
function Table({
  label,
  days,
  values,
  format,
}: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  format: (n: number) => string;
}) {
  return (
    <details className="text-xs">
      <summary className="w-fit cursor-pointer text-faint hover:text-muted">
        As a table
      </summary>
      <div className="mt-1 max-h-64 overflow-y-auto">
        <table className="w-full tabular-nums">
          <caption className="sr-only">{label}</caption>
          <tbody>
            {days
              .map((day, i) => (
                <tr key={day} className="border-b border-line last:border-0">
                  <td className="py-1 text-muted">
                    {weekday(day)} {day}
                  </td>
                  <td className="py-1 text-right">
                    {values[i] === undefined ? "—" : format(values[i]!)}
                  </td>
                </tr>
              ))
              .toReversed()}
          </tbody>
        </table>
      </div>
    </details>
  );
}

/** How wide a box is, as it changes. */
function useWidth(box: RefObject<HTMLDivElement | null>): number {
  const [width, setWidth] = useState(640);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    setWidth(el.clientWidth || 640);
    const watch = new ResizeObserver(([entry]) => {
      if (entry) setWidth(Math.round(entry.contentRect.width));
    });
    watch.observe(el);
    return () => watch.disconnect();
  }, [box]);
  return width;
}
