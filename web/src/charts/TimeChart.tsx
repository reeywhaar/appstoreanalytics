import {
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react";

import { labelled, ticks } from "@app/charts/scale";
import type { ChartMode, Series } from "@app/charts/series";
import { tick, weekday } from "@app/format";

/** The narrowest a day may get: a 6px column and its 2px gap. */
const minBand = 8;

/**
 * The surface gap between two segments of a stacked column: enough to tell two apps' colours
 * apart where they meet, too little to read as white bands across a short column.
 */
const gap = 1;

/** How round a column's corners are: enough to soften it, too little to read as a cap. */
const radius = 2;

/**
 * One measure over the days there are, a column a day or a line through them, scrolling back
 * through them.
 *
 * window is how many days fill the view: the range picked. The chart opens on the latest of
 * them, which is the one looked at first, and scrolls back through the rest under an axis that
 * stays. The scale is of the days in view, settled when the scrolling stops, so a quiet week is
 * not flattened by a busy one long ago. A day with no value is not known yet, which is not zero,
 * and is drawn as nothing. Every value is also in the table under it, so the hover is never the
 * only way to read one.
 *
 * Given several series, the apps are told apart: a column is each app's part stacked, in the
 * series' order, and a line is drawn for each. The legend naming them is the page's, above the
 * charts it serves.
 */
export function TimeChart({
  label,
  days,
  values,
  series,
  color = "var(--color-series)",
  mode = "bars",
  window: fill,
  format,
  cell = format,
  axis,
  whole = true,
  height = 168,
}: {
  label: string;
  days: string[];
  /** Each day's value: the sum of the series, when there are several. */
  values: (number | undefined)[];
  /** Each app's own values, to draw apart; one series or none draws values alone. */
  series?: Series[];
  /** The colour of values, drawn alone. */
  color?: string;
  mode?: ChartMode;
  /** How many days fill the view; all of them when left out. */
  window?: number;
  /** A value as the tooltip says it, and the table unless cell says otherwise. */
  format: (n: number) => string;
  /** A value as the table says it, under the chart's title that already names what it counts:
   *  12, not 12 downloads, on every row. */
  cell?: (n: number) => string;
  /** A value as the axis says it, shorter. */
  axis: (n: number) => string;
  whole?: boolean;
  height?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const width = useWidth(box);
  const [at, setAt] = useState<number | null>(null);
  // How far the strip is scrolled, for keeping the tooltip inside what is in view. Followed
  // only while a day is picked: a re-render on every scrolled frame is a cost with nothing to show.
  const [scrollX, setScrollX] = useState(0);
  const follow = () => setScrollX(scroller.current?.scrollLeft ?? 0);

  const many = series && series.length > 1 ? series : null;
  // What is drawn: each series, or the values as one.
  const drawn: Series[] = many ?? [
    { id: "", name: label, color: series?.[0]?.color ?? color, values },
  ];

  const n = days.length;
  const slots = Math.max(1, fill ?? n);
  // The days in view, as first and last index; the latest window of them until measured.
  const [seen, setSeen] = useState<[number, number] | null>(null);
  const [first, last] = seen ?? [Math.max(0, n - slots), n - 1];

  // Stacked, a column is as tall as its day's sum; as lines, the scale is of each line.
  const scaled = mode === "lines" ? drawn.map((s) => s.values) : [values];
  const known = scaled.flatMap((v) =>
    v.slice(first, last + 1).filter((x): x is number => x !== undefined),
  );
  const yTicks = ticks(Math.min(0, ...known), Math.max(0, ...known), 4, whole);
  const lo = yTicks[0]!;
  const hi = yTicks[yTicks.length - 1]!;
  // Wide enough for the axis of every day, not only those in view: a gutter that narrows as a
  // quiet stretch scrolls in changes the strip's width, and the strip opens on the latest day
  // again whenever that changes.
  const every = values.filter((v): v is number => v !== undefined);
  const widest = ticks(Math.min(0, ...every), Math.max(0, ...every), 4, whole);
  const left =
    Math.max(...[...yTicks, ...widest].map((t) => axis(t).length)) * 6.5 + 12;
  // Room past the last column for the last day's label, which is centred under it.
  const right = 18;
  const top = 8;
  const bottom = 22;
  const view = Math.max(10, width - left - right);
  const band = Math.max(minBand, view / slots);
  const plotW = Math.max(view, n * band);
  // Fewer days than fill the view stand at its right, where the latest day always is.
  const offset = plotW - n * band;
  const contentW = plotW + right;
  const plotH = height - top - bottom;
  const x = (i: number) => offset + band * i + band / 2;
  const y = (v: number) =>
    Math.max(0, top + plotH - ((v - lo) / (hi - lo || 1)) * plotH);
  const xLabels = labelled(n, Math.max(2, Math.floor(plotW / 64)));
  const barW = Math.min(24, Math.max(1, band - 2));

  /** Which days are in view, from where the scroller stands. */
  const measure = () => {
    const el = scroller.current;
    if (!el || n === 0) return;
    const a = Math.floor((el.scrollLeft - offset) / band);
    const b =
      Math.ceil((el.scrollLeft + el.clientWidth - right - offset) / band) - 1;
    const next: [number, number] = [
      Math.min(n - 1, Math.max(0, a)),
      Math.min(n - 1, Math.max(0, b)),
    ];
    setSeen((s) => (s && s[0] === next[0] && s[1] === next[1] ? s : next));
  };

  // Open on the latest day, and again whenever the days or the window are others.
  const latest = days[n - 1];
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el) el.scrollLeft = el.scrollWidth;
    setSeen(null);
  }, [latest, n, slots, contentW]);

  // The scale settles when the scrolling stops, rather than jumping under the finger.
  const settle = useRef<ReturnType<typeof setTimeout>>(undefined);
  const scrolled = () => {
    if (at !== null) follow();
    clearTimeout(settle.current);
    settle.current = setTimeout(measure, 120);
  };

  const pick = (e: PointerEvent<SVGRectElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const i = Math.floor((e.clientX - rect.left - offset) / band);
    setAt(Math.min(n - 1, Math.max(0, i)));
    follow();
  };

  /** Moves the day picked by keys, and scrolls it into view. */
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
    const next = Math.min(n - 1, Math.max(0, (at ?? n - 1) + move));
    setAt(next);
    const el = scroller.current;
    if (!el) return;
    const mid = x(next);
    if (mid < el.scrollLeft + band) el.scrollLeft = mid - band;
    if (mid > el.scrollLeft + el.clientWidth - band) {
      el.scrollLeft = mid - el.clientWidth + band;
    }
    follow();
  };

  // Over its day, but never past either edge of what is in view: the strip scrolls, and a
  // tooltip held only inside the whole of it is cut off by the edge of the part showing.
  const tipW = many ? 200 : 140;
  const viewL = scrollX;
  const viewR = Math.min(contentW, scrollX + Math.max(tipW, width - left));
  const said =
    at === null
      ? null
      : {
          at,
          day: days[at]!,
          value: values[at],
          left: Math.max(viewL, Math.min(x(at) - tipW / 2, viewR - tipW)),
        };

  return (
    <div className="flex flex-col gap-1">
      <div
        ref={box}
        // A tap picks a day: not text to select, a word to look up, or a double tap to zoom.
        className="relative flex touch-manipulation rounded-md outline-offset-2 select-none [-webkit-tap-highlight-color:transparent] [-webkit-touch-callout:none] focus-visible:outline-2 focus-visible:outline-brand"
        tabIndex={0}
        role="group"
        aria-label={`${label}. Arrow keys move from day to day.`}
        onKeyDown={step}
        onFocus={() => {
          setAt((i) => i ?? n - 1);
          follow();
        }}
        onBlur={() => setAt(null)}
      >
        <svg
          width={left}
          height={height}
          className="block shrink-0"
          aria-hidden="true"
        >
          {yTicks.map((t) => (
            <text
              key={t}
              x={left - 8}
              y={y(t)}
              dy="0.32em"
              textAnchor="end"
              className="fill-faint text-[11px] tabular-nums"
            >
              {axis(t)}
            </text>
          ))}
        </svg>
        <div
          ref={scroller}
          onScroll={scrolled}
          className="scroll-thin min-w-0 flex-1 overflow-x-auto overflow-y-hidden"
        >
          <div className="relative" style={{ width: contentW }}>
            <svg
              width={contentW}
              height={height}
              className="block"
              aria-hidden="true"
            >
              {yTicks.map((t) => (
                <line
                  key={t}
                  x1={0}
                  x2={plotW}
                  y1={y(t)}
                  y2={y(t)}
                  className={t === 0 ? "stroke-axis" : "stroke-grid"}
                  strokeWidth={1}
                  shapeRendering="crispEdges"
                />
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
              {mode === "bars" ? (
                days.map((day, i) => (
                  <g
                    key={day}
                    className="transition-opacity"
                    opacity={at === null || at === i ? 1 : 0.45}
                  >
                    {stack(drawn, i).map((part) => (
                      <path
                        key={part.id}
                        d={segment(
                          x(i),
                          barW,
                          y(part.from),
                          y(part.to),
                          part.outer,
                        )}
                        style={{ fill: part.color }}
                      />
                    ))}
                  </g>
                ))
              ) : (
                <>
                  {at !== null ? (
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
                  {drawn.map((s) => (
                    <path
                      key={s.id}
                      d={line(s.values, x, y)}
                      fill="none"
                      strokeWidth={2}
                      strokeLinejoin="round"
                      strokeLinecap="round"
                      style={{ stroke: s.color }}
                    />
                  ))}
                  {at !== null
                    ? drawn.map((s) =>
                        s.values[at] === undefined ? null : (
                          <circle
                            key={s.id}
                            cx={x(at)}
                            cy={y(s.values[at]!)}
                            r={4}
                            strokeWidth={2}
                            className="stroke-bg"
                            style={{ fill: s.color }}
                          />
                        ),
                      )
                    : null}
                </>
              )}
              <rect
                x={0}
                y={top}
                width={plotW}
                height={plotH}
                fill="transparent"
                onPointerMove={pick}
                onPointerDown={pick}
                // A finger lifting is a pointer leaving, and the day it tapped stays picked
                // until a tap elsewhere takes focus away.
                onPointerLeave={(e) => {
                  if (e.pointerType === "mouse") setAt(null);
                }}
              />
            </svg>
          </div>
        </div>
        {/* Over the strip rather than in it, so a tall one is not cut off by the strip's own
            edge: it may reach over what is under the chart. */}
        {said ? (
          <div
            className="pointer-events-none absolute top-0 z-10 rounded-md border border-line bg-bg px-2.5 py-1.5 text-xs shadow-sm"
            style={{ left: left + said.left - scrollX, width: tipW }}
          >
            <div className="text-sm font-semibold text-fg tabular-nums">
              {said.value === undefined
                ? "Not reported yet"
                : format(said.value)}
            </div>
            <div className="text-muted">
              {weekday(said.day)} {said.day}
            </div>
            {many && said.value !== undefined ? (
              <ul className="mt-1 flex flex-col gap-0.5 border-t border-line pt-1">
                {many.map((s) => (
                  <li key={s.id} className="flex items-center gap-1.5">
                    <Key color={s.color} />
                    <span className="font-semibold text-fg tabular-nums">
                      {s.values[said.at] === undefined
                        ? "—"
                        : cell(s.values[said.at]!)}
                    </span>
                    <span className="min-w-0 truncate text-muted">
                      {s.name}
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        ) : null}
        <p className="sr-only" aria-live="polite">
          {said
            ? `${said.day}: ${said.value === undefined ? "not reported yet" : format(said.value)}`
            : ""}
        </p>
      </div>
      <Table
        label={label}
        days={days}
        values={values}
        series={many}
        format={cell}
      />
    </div>
  );
}

/** A short stroke of a series' colour: a tooltip's key, lighter than a box. */
function Key({ color }: { color: string }) {
  return (
    <svg width={10} height={2} className="shrink-0" aria-hidden="true">
      <rect width={10} height={2} rx={1} style={{ fill: color }} />
    </svg>
  );
}

/** A part of a day's column: from one value to another, in a series' colour. */
type Part = {
  id: string;
  color: string;
  from: number;
  to: number;
  /** The part at the column's end; the rest stop a gap short of the part above. */
  outer: boolean;
};

/**
 * A day's column as each series' part of it, from the baseline out, in the series' order: what
 * is above zero stacked up, what is below it stacked down. A part of nothing draws nothing.
 */
function stack(series: Series[], i: number): Part[] {
  const up: Part[] = [];
  const down: Part[] = [];
  let above = 0;
  let below = 0;
  for (const s of series) {
    const v = s.values[i];
    if (v === undefined || v === 0) continue;
    if (v > 0) {
      up.push({
        id: s.id,
        color: s.color,
        from: above,
        to: above + v,
        outer: false,
      });
      above += v;
    } else {
      down.push({
        id: s.id,
        color: s.color,
        from: below,
        to: below + v,
        outer: false,
      });
      below += v;
    }
  }
  if (up.length) up[up.length - 1]!.outer = true;
  if (down.length) down[down.length - 1]!.outer = true;
  return [...up, ...down];
}

/**
 * A part of a column, from base to end, short of the next part by the surface gap, which is what
 * separates them rather than a stroke. The column's end is softened; where parts meet, and where
 * the column stands on the baseline, it is square, so the gap reads as one hairline across it. A
 * part too short for its gap keeps a pixel of its colour rather than leaving only the gap.
 */
function segment(
  cx: number,
  w: number,
  base: number,
  end: number,
  outer: boolean,
): string {
  const h = Math.abs(end - base);
  if (h < 0.5) return "";
  // From the end towards the base; the other way is -dir.
  const dir = end < base ? 1 : -1;
  const short = outer || h <= gap + 1 ? 0 : gap;
  const tip = base - dir * Math.max(1, h - short);
  const l = cx - w / 2;
  const rr = cx + w / 2;
  if (!outer) return `M${l},${base}V${tip}H${rr}V${base}Z`;
  const r = Math.min(radius, w / 2, Math.abs(tip - base));
  return `M${l},${base}V${tip + dir * r}Q${l},${tip} ${l + r},${tip}H${rr - r}Q${rr},${tip} ${rr},${tip + dir * r}V${base}Z`;
}

/** A line through a series' days, lifted over a day not known yet rather than drawn to zero. */
function line(
  values: (number | undefined)[],
  x: (i: number) => number,
  y: (v: number) => number,
): string {
  let d = "";
  let down = false;
  values.forEach((v, i) => {
    if (v === undefined) {
      down = false;
      return;
    }
    d += `${down ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`;
    down = true;
  });
  return d;
}

/**
 * The same values as a table, for whoever cannot or would rather not read them off a chart:
 * with several series, a column each after the day's sum.
 */
function Table({
  label,
  days,
  values,
  series,
  format,
}: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  series: Series[] | null;
  format: (n: number) => string;
}) {
  const say = (v: number | undefined) => (v === undefined ? "—" : format(v));
  return (
    <details className="text-xs">
      <summary className="w-fit cursor-pointer text-faint hover:text-muted">
        As a table
      </summary>
      <div className="mt-1 max-h-64 overflow-auto">
        <table className="w-full tabular-nums">
          <caption className="sr-only">{label}</caption>
          {series ? (
            <thead>
              <tr className="border-b border-line text-muted">
                <th className="py-1 text-left font-normal">Day</th>
                <th className="py-1 pl-3 text-right font-normal">All</th>
                {series.map((s) => (
                  <th
                    key={s.id}
                    className="py-1 pl-3 text-right font-normal whitespace-nowrap"
                  >
                    {s.name}
                  </th>
                ))}
              </tr>
            </thead>
          ) : null}
          <tbody>
            {days
              .map((day, i) => (
                <tr key={day} className="border-b border-line last:border-0">
                  <td className="py-1 whitespace-nowrap text-muted">
                    {weekday(day)} {day}
                  </td>
                  <td className="py-1 pl-3 text-right">{say(values[i])}</td>
                  {series?.map((s) => (
                    <td key={s.id} className="py-1 pl-3 text-right">
                      {say(s.values[i])}
                    </td>
                  ))}
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
