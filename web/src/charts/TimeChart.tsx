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

/** The narrowest a day may get: a 6px column and its 2px gap. */
const minBand = 8;

/**
 * One measure over the days there are, a column a day, scrolling back through them.
 *
 * window is how many days fill the view: the range picked. The chart opens on the latest of
 * them, which is the one looked at first, and scrolls back through the rest under an axis that
 * stays. The scale is of the days in view, settled when the scrolling stops, so a quiet week is
 * not flattened by a busy one long ago. A day with no value is not known yet, which is not zero,
 * and is drawn as nothing. One series, so no legend: the card's title names it. Every value is
 * also in the table under it, so the hover is never the only way to read one.
 */
export function TimeChart({
  label,
  days,
  values,
  window: fill,
  format,
  axis,
  whole = true,
  height = 168,
}: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  /** How many days fill the view; all of them when left out. */
  window?: number;
  /** A value as the tooltip and the table say it. */
  format: (n: number) => string;
  /** A value as the axis says it, shorter. */
  axis: (n: number) => string;
  whole?: boolean;
  height?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const width = useWidth(box);
  const [at, setAt] = useState<number | null>(null);

  const n = days.length;
  const slots = Math.max(1, fill ?? n);
  // The days in view, as first and last index; the latest window of them until measured.
  const [seen, setSeen] = useState<[number, number] | null>(null);
  const [first, last] = seen ?? [Math.max(0, n - slots), n - 1];

  const known = values
    .slice(first, last + 1)
    .filter((v): v is number => v !== undefined);
  const yTicks = ticks(Math.min(0, ...known), Math.max(0, ...known), 4, whole);
  const lo = yTicks[0]!;
  const hi = yTicks[yTicks.length - 1]!;
  const left = Math.max(...yTicks.map((t) => axis(t).length)) * 6.5 + 12;
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
  const base = y(0);
  const xLabels = labelled(n, Math.max(2, Math.floor(plotW / 64)));

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
    clearTimeout(settle.current);
    settle.current = setTimeout(measure, 120);
  };

  const pick = (e: PointerEvent<SVGRectElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const i = Math.floor((e.clientX - rect.left - offset) / band);
    setAt(Math.min(n - 1, Math.max(0, i)));
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
  };

  const said =
    at === null
      ? null
      : {
          day: days[at]!,
          value: values[at],
          left: Math.min(Math.max(0, x(at) - 70), Math.max(0, contentW - 140)),
        };

  return (
    <div className="flex flex-col gap-1">
      <div
        ref={box}
        // A tap picks a day: not text to select, a word to look up, or a double tap to zoom.
        className="flex touch-manipulation rounded-md outline-offset-2 select-none [-webkit-tap-highlight-color:transparent] [-webkit-touch-callout:none] focus-visible:outline-2 focus-visible:outline-brand"
        tabIndex={0}
        role="group"
        aria-label={`${label}. Arrow keys move from day to day.`}
        onKeyDown={step}
        onFocus={() => setAt((i) => i ?? n - 1)}
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
              {values.map((v, i) =>
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
          </div>
        </div>
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
