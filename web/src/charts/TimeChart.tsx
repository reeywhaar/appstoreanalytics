import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";

import { labelled, ticks } from "@app/charts/scale";
import type { ChartMode, Series } from "@app/charts/series";
import { BarList } from "@app/charts/BarList";
import { useTimeGraph } from "@app/charts/TimeGraph";
import { Dialog } from "@app/components/Dialog";
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
 * The least of its colour a part of something shows, so a day's one purchase stacked on hundreds
 * of updates is still there to see: a sliver, not a cap of its own.
 */
const least = 1.5;

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
  onEarlier,
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
  /** Asks for the days before the earliest there is, as the strip nears its start. */
  onEarlier?: () => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const width = useWidth(box);
  const [at, setAt] = useState<number | null>(null);
  // How far the strip is scrolled: which days to draw, and where the tooltip fits. Followed a
  // frame at a time, since only the days in view and either side of it are drawn.
  const [scrollX, setScrollX] = useState(0);
  const follow = () => setScrollX(scroller.current?.scrollLeft ?? 0);
  const frame = useRef(0);
  // The page's charts, which show one readout at a time and scroll as one.
  const graph = useTimeGraph();
  const [self] = useState(() => Symbol(label));

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

  // Only the days in view are drawn, and a view's width either side of it, so a scroll never
  // shows a blank before the next frame does: a history is a thousand days and more, and drawing
  // each of them, on every chart, on every hover, was most of what the page cost.
  const viewW = Math.max(1, width - left);
  const drawFrom = Math.max(0, Math.floor((scrollX - viewW - offset) / band));
  const drawTo = Math.min(
    n - 1,
    Math.ceil((scrollX + 2 * viewW - offset) / band),
  );
  const shownDays: number[] = [];
  for (let i = drawFrom; i <= drawTo; i++) shownDays.push(i);

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

  // A scroll the strip was given, to open or to follow the others, rather than one somebody made:
  // where it was sent, so the scroll it fires is not passed on to the others in turn.
  const sent = useRef<number | null>(null);
  /** Scrolls the strip to a place of its own accord. */
  const sendTo = (el: HTMLDivElement, to: number) => {
    const target = Math.min(el.scrollWidth - el.clientWidth, Math.max(0, to));
    if (Math.abs(el.scrollLeft - target) < 1) return;
    sent.current = target;
    el.scrollLeft = target;
  };
  /** How many days back from the latest the right edge of the view stands. */
  const daysBack = (el: HTMLDivElement) =>
    (el.scrollWidth - el.clientWidth - el.scrollLeft) / band;
  /** Where the view stands that many days back from the latest. */
  const backTo = (el: HTMLDivElement, back: number) =>
    el.scrollWidth - el.clientWidth - back * band;

  // Open on the latest day, and again whenever the days or the window are others. Days that come
  // before the earliest, the year before read as the strip neared its start, leave the days in
  // view where they were, and the day picked and the days measured with them.
  const latest = days[n - 1];
  const earliest = days[0];
  const was = useRef({ latest, earliest, n, offset, slots });
  useLayoutEffect(() => {
    const el = scroller.current;
    const before = was.current;
    was.current = { latest, earliest, n, offset, slots };
    if (!el) return;
    const prepended =
      before.latest === latest &&
      before.slots === slots &&
      earliest !== undefined &&
      before.earliest !== undefined &&
      earliest < before.earliest;
    if (prepended) {
      const added = n - before.n;
      sendTo(el, el.scrollLeft + added * band + (offset - before.offset));
      setSeen((s) => (s ? [s[0] + added, s[1] + added] : s));
      setAt((i) => (i === null ? i : i + added));
    } else {
      // Where the others stand among these days, for a chart drawn after they were scrolled;
      // the latest day otherwise.
      const back = latest === undefined ? null : graph.standing(latest, slots);
      sendTo(el, back === null ? el.scrollWidth : backTo(el, back));
      setSeen(null);
    }
    follow();
    // band and offset follow from n, slots and contentW, which are listed.
  }, [latest, earliest, n, slots, contentW]);

  // Scrolled with the others: to the same days, which is the same days back from the latest.
  useEffect(
    () =>
      graph.follow(self, (others) => {
        const el = scroller.current;
        if (!el || others.latest !== latest || others.slots !== slots) return;
        sendTo(el, backTo(el, others.back));
      }),
    // sendTo and backTo follow from band, which is listed.
    [graph, self, latest, slots, band],
  );

  // The scale settles when the scrolling stops, rather than jumping under the finger.
  const settle = useRef<ReturnType<typeof setTimeout>>(undefined);
  const scrolled = () => {
    cancelAnimationFrame(frame.current);
    frame.current = requestAnimationFrame(follow);
    clearTimeout(settle.current);
    settle.current = setTimeout(measure, 120);
    const el = scroller.current;
    if (!el) return;
    // Somebody's scroll takes the others with it; one the strip was given is theirs already.
    const given =
      sent.current !== null && Math.abs(el.scrollLeft - sent.current) < 1;
    sent.current = null;
    if (!given && latest !== undefined) {
      graph.scrolled(self, { latest, slots, back: daysBack(el) });
    }
    // A mouse resting on the strip as it scrolls is over another day, with no move to say so:
    // that day is the one it reads. A day picked otherwise goes along with the strip, and is let
    // go once it is out of view, rather than read beside days it is not among.
    if (hover.current !== null) setAt(dayAt(hover.current));
    else {
      setAt((i) =>
        i === null ||
        (x(i) >= el.scrollLeft && x(i) <= el.scrollLeft + el.clientWidth)
          ? i
          : null,
      );
    }
    // Within a view of the start: the year before is asked for while there is still a view of
    // days to scroll through, so it is there before the strip runs out.
    if (el.scrollLeft < el.clientWidth) onEarlier?.();
  };

  // A finger down on a chart is as likely the start of a scroll as a tap. A mouse picks the day it
  // is over as it moves; a finger only on lifting where it came down, which a scroll never does:
  // the browser takes a moving finger for scrolling and cancels the press.
  const press = useRef<{ x: number; y: number } | null>(null);
  // A focus that follows a press is the pointer's; one with none is the keyboard's, which is the
  // only one that picks a day of its own accord.
  const pressed = useRef(false);
  // Where a mouse over the strip is across the window; none while it is elsewhere.
  const hover = useRef<number | null>(null);
  // How the day was picked, which is how the readout says to see the rest of it.
  const via = useRef<"mouse" | "touch" | "keys">("mouse");

  // The day whose every app is shown in a dialog of its own: clicked, tapped a second time, or
  // entered. Only where there are apps to tell apart.
  const [detail, setDetail] = useState<number | null>(null);
  // Closed, the dialog hands focus back to the chart, which is not the keyboard arriving.
  const back = useRef(false);
  const open = (i: number) => {
    if (!many) return;
    back.current = true;
    setDetail(i);
  };

  /** The day under a point across the window. */
  const dayAt = (clientX: number) => {
    const el = scroller.current;
    const start = el ? el.getBoundingClientRect().left - el.scrollLeft : 0;
    const i = Math.floor((clientX - start - offset) / band);
    return Math.min(n - 1, Math.max(0, i));
  };

  const pick = (e: PointerEvent<SVGRectElement>) => {
    hover.current = e.pointerType === "mouse" ? e.clientX : null;
    via.current = e.pointerType === "mouse" ? "mouse" : "touch";
    setAt(dayAt(e.clientX));
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
    if ((e.key === "Enter" || e.key === " ") && at !== null && many) {
      e.preventDefault();
      open(at);
      return;
    }
    const move = moves[e.key];
    if (move === undefined) return;
    e.preventDefault();
    const next = Math.min(n - 1, Math.max(0, (at ?? n - 1) + move));
    via.current = "keys";
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

  // The readout of the day picked is the page's, not the chart's: above the chart, or under it
  // where the window has no room above, over its day, so it never covers what is being read.
  // Placed once its size is known, before it is painted, and again as the page or strip moves.
  const tipW = many ? 200 : 140;
  const said = at === null ? null : { at, day: days[at]!, value: values[at] };
  const listed =
    many && said && said.value !== undefined ? readout(many, said.at) : null;

  // One readout on the page at a time: picking a day takes the page's layer from whichever chart
  // had it, and lets go of it when the day is let go.
  const picked = at !== null;
  useEffect(() => {
    if (picked) graph.claim(self, () => setAt(null));
    else graph.release(self);
  }, [graph, picked, self]);
  useEffect(() => () => graph.release(self), [graph, self]);

  // The day picked here is lit on the page's other charts, and the day picked on another is lit
  // here: the same day dims the rest everywhere, with the readout only where it was picked.
  const [echo, setEcho] = useState<string | null>(null);
  useEffect(() => graph.lit(self, setEcho), [graph, self]);
  const pickedDay = at === null ? null : days[at]!;
  useEffect(() => graph.light(self, pickedDay), [graph, self, pickedDay]);
  useEffect(() => () => graph.light(self, null), [graph, self]);
  const echoed = echo === null ? -1 : days.indexOf(echo);
  const lit = at ?? (echoed < 0 ? null : echoed);

  const tip = useRef<HTMLDivElement>(null);
  const [place, setPlace] = useState<{ left: number; top: number } | null>(
    null,
  );
  const dayX = at === null ? 0 : x(at);
  useLayoutEffect(() => {
    if (at === null) {
      setPlace(null);
      return;
    }
    const position = () => {
      const strip = scroller.current;
      const chart = box.current?.getBoundingClientRect();
      const el = tip.current;
      if (!strip || !chart || !el) return;
      const margin = 8;
      const w = el.offsetWidth;
      const h = el.offsetHeight;
      const across = document.documentElement.clientWidth;
      const down = window.innerHeight;
      const centre =
        strip.getBoundingClientRect().left + dayX - strip.scrollLeft;
      const atLeft = Math.min(
        Math.max(margin, centre - w / 2),
        across - w - margin,
      );
      const above = chart.top - margin - h;
      const below = chart.bottom + margin;
      // Above if it fits, under if that fits, and otherwise on the roomier side, kept in view.
      const atTop =
        above >= margin
          ? above
          : below + h <= down - margin
            ? below
            : chart.top > down - chart.bottom
              ? Math.max(margin, above)
              : Math.min(below, down - h - margin);
      setPlace((p) =>
        p && p.left === atLeft && p.top === atTop
          ? p
          : { left: atLeft, top: atTop },
      );
    };
    position();
    // Capturing: the strip's own scroll does not bubble, and moves its day across the window.
    window.addEventListener("scroll", position, true);
    window.addEventListener("resize", position);
    return () => {
      window.removeEventListener("scroll", position, true);
      window.removeEventListener("resize", position);
    };
  }, [at, dayX]);

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
        onPointerDownCapture={() => {
          pressed.current = true;
        }}
        onFocus={() => {
          if (pressed.current || back.current) {
            back.current = false;
            return;
          }
          via.current = "keys";
          setAt((i) => i ?? n - 1);
          follow();
        }}
        onBlur={() => {
          pressed.current = false;
          setAt(null);
        }}
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
              {shownDays.map((i) =>
                xLabels.has(i) ? (
                  <text
                    key={days[i]}
                    x={x(i)}
                    y={top + plotH + 16}
                    textAnchor="middle"
                    className="fill-faint text-[11px] tabular-nums"
                  >
                    {tick(days[i]!)}
                  </text>
                ) : null,
              )}
              {mode === "bars" ? (
                shownDays.map((i) => (
                  <g
                    key={days[i]}
                    className="transition-opacity"
                    opacity={lit === null || lit === i ? 1 : 0.45}
                  >
                    {stack(drawn, i, y).map((part) => (
                      <path
                        key={part.id}
                        d={segment(x(i), barW, part.from, part.to, part.outer)}
                        style={{ fill: part.color }}
                      />
                    ))}
                  </g>
                ))
              ) : (
                <>
                  {lit !== null ? (
                    <line
                      x1={x(lit)}
                      x2={x(lit)}
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
                      d={line(s.values, x, y, drawFrom - 1, drawTo + 1)}
                      fill="none"
                      strokeWidth={2}
                      strokeLinejoin="round"
                      strokeLinecap="round"
                      style={{ stroke: s.color }}
                    />
                  ))}
                  {lit !== null
                    ? drawn.map((s) =>
                        s.values[lit] === undefined ? null : (
                          <circle
                            key={s.id}
                            cx={x(lit)}
                            cy={y(s.values[lit]!)}
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
                className={many ? "cursor-pointer" : undefined}
                onPointerMove={(e) => {
                  if (e.pointerType === "mouse") pick(e);
                }}
                onPointerDown={(e) => {
                  if (e.pointerType === "mouse") pick(e);
                  else press.current = { x: e.clientX, y: e.clientY };
                }}
                // A click shows the day's every app; a tap picks the day first, and a second
                // tap on it shows them, since a finger has no hover to read it by before.
                onPointerUp={(e) => {
                  if (e.pointerType === "mouse") {
                    if (e.button === 0) open(dayAt(e.clientX));
                    return;
                  }
                  const p = press.current;
                  press.current = null;
                  if (p && Math.hypot(e.clientX - p.x, e.clientY - p.y) < 10) {
                    const i = dayAt(e.clientX);
                    if (i === at) open(i);
                    else pick(e);
                  }
                }}
                onPointerCancel={() => {
                  press.current = null;
                }}
                // A finger lifting is a pointer leaving, and the day it tapped stays picked
                // until a tap elsewhere takes focus away.
                onPointerLeave={(e) => {
                  if (e.pointerType !== "mouse") return;
                  hover.current = null;
                  setAt(null);
                }}
              />
            </svg>
          </div>
        </div>
        {said
          ? createPortal(
              <div
                ref={tip}
                className="pointer-events-none fixed z-50 rounded-md border border-line bg-bg px-2.5 py-1.5 text-xs shadow-sm"
                style={{
                  left: place?.left ?? 0,
                  top: place?.top ?? 0,
                  width: tipW,
                  visibility: place ? "visible" : "hidden",
                }}
              >
                <div className="text-sm font-semibold text-fg tabular-nums">
                  {said.value === undefined
                    ? "Not reported yet"
                    : format(said.value)}
                </div>
                <div className="text-muted">
                  {weekday(said.day)} {said.day}
                </div>
                {listed && listed.shown.length > 0 ? (
                  <ul className="mt-1 flex flex-col gap-0.5 border-t border-line pt-1">
                    {listed.shown.map((r) => (
                      <li key={r.id} className="flex items-center gap-1.5">
                        <Key color={r.color} />
                        <span className="font-semibold text-fg tabular-nums">
                          {cell(r.value)}
                        </span>
                        <span className="min-w-0 truncate text-muted">
                          {r.name}
                        </span>
                      </li>
                    ))}
                    {listed.rest.length > 0 ? (
                      <li className="flex items-center gap-1.5">
                        <Key color="var(--color-faint)" />
                        <span className="font-semibold text-fg tabular-nums">
                          {cell(listed.rest.reduce((t, r) => t + r.value, 0))}
                        </span>
                        <span className="min-w-0 truncate text-muted">
                          {listed.rest.length} others
                        </span>
                      </li>
                    ) : null}
                  </ul>
                ) : null}
                {listed && listed.rest.length > 0 ? (
                  <div className="mt-1 text-faint">
                    {via.current === "touch"
                      ? "Tap again for every app"
                      : via.current === "keys"
                        ? "Enter for every app"
                        : "Click for every app"}
                  </div>
                ) : null}
              </div>,
              graph.layer ?? document.body,
            )
          : null}
        <p className="sr-only" aria-live="polite">
          {said
            ? `${said.day}: ${said.value === undefined ? "not reported yet" : format(said.value)}`
            : ""}
        </p>
      </div>
      {/* Beside the chart rather than in it: focus moving into the dialog is not the chart's. */}
      {many ? (
        <Dialog
          open={detail !== null}
          onClose={() => setDetail(null)}
          title={label}
          aside={
            detail !== null
              ? `${weekday(days[detail]!)} ${days[detail]}`
              : undefined
          }
        >
          {detail !== null ? (
            <DayApps
              label={`${label}, ${days[detail]}`}
              shares={ranked(many, detail)}
              total={values[detail]}
              format={format}
              cell={cell}
            />
          ) : null}
        </Dialog>
      ) : null}
      <Table
        label={label}
        days={days}
        values={values}
        series={many}
        format={cell}
        onEarlier={onEarlier}
      />
    </div>
  );
}

/** How many apps a readout names; past one more than that, the rest are one line. */
const named = 7;

/** An app's part of a day. */
type Share = { id: string; name: string; color: string; value: number };

/**
 * The apps with anything on a day, the largest first, ties in the series' order. An app with
 * nothing that day is left out, so what is listed is what made the day.
 */
function ranked(series: Series[], i: number): Share[] {
  return series
    .flatMap((s, at) => {
      const value = s.values[i];
      return value === undefined || value === 0
        ? []
        : [{ at, share: { id: s.id, name: s.name, color: s.color, value } }];
    })
    .toSorted(
      (a, b) =>
        Math.abs(b.share.value) - Math.abs(a.share.value) || a.at - b.at,
    )
    .map((r) => r.share);
}

/** A day's apps as a readout lists them: the largest named, and the rest as one, past a few. */
function readout(series: Series[], i: number) {
  const all = ranked(series, i);
  // One more is named rather than made a line of its own as one other.
  if (all.length <= named + 1) return { shown: all, rest: [] as Share[] };
  return { shown: all.slice(0, named), rest: all.slice(named) };
}

/** Every app with anything on a day, ranked, with what each was of the day. */
function DayApps({
  label,
  shares,
  total,
  format,
  cell,
}: {
  label: string;
  shares: Share[];
  total: number | undefined;
  format: (n: number) => string;
  cell: (n: number) => string;
}) {
  if (total === undefined) {
    return <p className="text-sm text-muted">Not reported yet.</p>;
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="text-2xl font-semibold tabular-nums">{format(total)}</div>
      {shares.length > 0 ? (
        <BarList
          label={label}
          named="App"
          limit={shares.length}
          columns={[
            { label: "Value", format: cell },
            {
              label: "Share",
              format: (n) => `${n < 1 && n > 0 ? "<1" : Math.round(n)}%`,
            },
          ]}
          rows={shares.map((s) => ({
            id: s.id,
            text: s.name,
            name: (
              <span className="flex items-center gap-1.5">
                <Key color={s.color} />
                <span className="truncate">{s.name}</span>
              </span>
            ),
            values: [s.value, total > 0 ? (s.value / total) * 100 : 0],
          }))}
        />
      ) : (
        <p className="text-sm text-muted">No app had any that day.</p>
      )}
    </div>
  );
}

/** A square of a series' colour, a tooltip's key: a stroke as thin as a line is too little of a
 *  colour to tell one from the next. */
function Key({ color }: { color: string }) {
  return (
    <svg width={8} height={8} className="shrink-0" aria-hidden="true">
      <rect width={8} height={8} rx={2} style={{ fill: color }} />
    </svg>
  );
}

/** A part of a day's column: from one pixel to another, in a series' colour. */
type Part = {
  id: string;
  color: string;
  from: number;
  to: number;
  /** The part at the column's end; the rest stop a gap short of the part above. */
  outer: boolean;
};

/** A series' part of a day before it is placed: how many pixels its value comes to. */
type Size = { id: string; color: string; size: number };

/**
 * A day's column as each series' part of it, from the baseline out, in the series' order: what
 * is above zero stacked up, what is below it stacked down. A part of nothing draws nothing.
 */
function stack(series: Series[], i: number, y: (v: number) => number): Part[] {
  const zero = y(0);
  const up: Size[] = [];
  const down: Size[] = [];
  for (const s of series) {
    const v = s.values[i];
    if (v === undefined || v === 0) continue;
    (v > 0 ? up : down).push({
      id: s.id,
      color: s.color,
      size: Math.abs(y(v) - zero),
    });
  }
  return [...pile(up, zero, -1), ...pile(down, zero, 1)];
}

/**
 * Parts laid end to end from the baseline, each long enough for `least` of its colour and, below
 * the column's end, the gap past it. What that lends the small ones is taken back from the
 * longest, so the column still ends at its total wherever the total leaves room for every part's
 * least.
 */
function pile(parts: Size[], zero: number, dir: 1 | -1): Part[] {
  const mins = parts.map((_, n) => least + (n === parts.length - 1 ? 0 : gap));
  const sizes = parts.map((p, n) => Math.max(mins[n]!, p.size));
  const total = parts.reduce((t, p) => t + p.size, 0);
  const floor = mins.reduce((t, m) => t + m, 0);
  let over = sizes.reduce((t, s) => t + s, 0) - Math.max(total, floor);
  while (over > 0.01) {
    const spare = sizes.map((s, n) => s - mins[n]!);
    const k = spare.indexOf(Math.max(...spare));
    const take = Math.min(over, sizes[k]! - mins[k]!);
    if (take <= 0) break;
    sizes[k]! -= take;
    over -= take;
  }
  let at = zero;
  return parts.map((p, n) => {
    const from = at;
    at += dir * sizes[n]!;
    return {
      id: p.id,
      color: p.color,
      from,
      to: at,
      outer: n === parts.length - 1,
    };
  });
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

/**
 * A line through a series' days from one to another, lifted over a day not known yet rather
 * than drawn to zero.
 */
function line(
  values: (number | undefined)[],
  x: (i: number) => number,
  y: (v: number) => number,
  from: number,
  to: number,
): string {
  let d = "";
  let down = false;
  for (let i = Math.max(0, from); i <= Math.min(values.length - 1, to); i++) {
    const v = values[i];
    if (v === undefined) {
      down = false;
      continue;
    }
    d += `${down ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`;
    down = true;
  }
  return d;
}

/**
 * The same values as a table, for whoever cannot or would rather not read them off a chart:
 * with several series, a column each after the day's sum, under a header that stays.
 *
 * Nothing is rendered until it is opened, and then only the rows in its window and a few either
 * side: a history is a thousand days and more, and every chart has a table.
 */
function Table(props: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  series: Series[] | null;
  format: (n: number) => string;
  onEarlier?: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <details
      className="text-xs"
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary className="w-fit cursor-pointer text-faint hover:text-muted">
        As a table
      </summary>
      {open ? <Rows {...props} /> : null}
    </details>
  );
}

/** How many rows past the window are rendered either side, so a scroll does not show a blank. */
const spare = 10;

function Rows({
  label,
  days,
  values,
  series,
  format,
  onEarlier,
}: {
  label: string;
  days: string[];
  values: (number | undefined)[];
  series: Series[] | null;
  format: (n: number) => string;
  onEarlier?: () => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const sample = useRef<HTMLTableRowElement>(null);
  const [top, setTop] = useState(0);
  // Measured from a row, not assumed: a spacer a pixel off a row is a thousand pixels off at the
  // end of a history.
  const [rowH, setRowH] = useState(25);
  const [boxH, setBoxH] = useState(256);
  useLayoutEffect(() => {
    const h = sample.current?.getBoundingClientRect().height;
    if (h) setRowH(h);
    if (box.current) setBoxH(box.current.clientHeight || 256);
  }, []);

  const say = (v: number | undefined) => (v === undefined ? "—" : format(v));
  const total = days.length;
  // Newest first: row r is the day total - 1 - r.
  const first = Math.max(0, Math.floor(top / rowH) - spare);
  const last = Math.min(total, Math.ceil((top + boxH) / rowH) + spare);
  const rows: number[] = [];
  for (let r = first; r < last; r++) rows.push(r);
  const columns = 2 + (series?.length ?? 0);

  return (
    <div
      ref={box}
      className="mt-1 max-h-64 overflow-auto"
      onScroll={(e) => {
        const el = e.currentTarget;
        setTop(el.scrollTop);
        // Within a window of the end, the earliest day: the year before is asked for, as the
        // chart asks for it nearing its start.
        if (
          el.scrollHeight - el.scrollTop - el.clientHeight <
          el.clientHeight
        ) {
          onEarlier?.();
        }
      }}
    >
      <table className="w-full tabular-nums" aria-rowcount={total + 1}>
        <caption className="sr-only">{label}</caption>
        {series ? (
          <thead className="sticky top-0 z-20 bg-bg">
            <tr className="border-b border-line text-muted">
              <th className="sticky left-0 z-10 bg-bg py-1 pr-3 text-left font-normal">
                Day
              </th>
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
          {first > 0 ? (
            <tr aria-hidden="true" style={{ height: first * rowH }}>
              <td colSpan={columns} />
            </tr>
          ) : null}
          {rows.map((r) => {
            const i = total - 1 - r;
            return (
              <tr
                key={days[i]}
                ref={r === first ? sample : undefined}
                aria-rowindex={r + 2}
                className="border-b border-line last:border-0"
              >
                {/* The day stays as the values scroll across under it, so a row is always its day's. */}
                <td className="sticky left-0 bg-bg py-1 pr-3 whitespace-nowrap text-muted">
                  {weekday(days[i]!)} {days[i]}
                </td>
                <td className="py-1 pl-3 text-right">{say(values[i])}</td>
                {series?.map((s) => (
                  <td key={s.id} className="py-1 pl-3 text-right">
                    {say(s.values[i])}
                  </td>
                ))}
              </tr>
            );
          })}
          {last < total ? (
            <tr aria-hidden="true" style={{ height: (total - last) * rowH }}>
              <td colSpan={columns} />
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
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
