import {
  createContext,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

/**
 * Where the charts stand: how many days back from the latest the right edge of the view is, of
 * the days that end on latest, a window of slots at a time. In days rather than pixels, since each
 * chart's axis is as wide as its own figures, so the same scroll is a different day in each.
 */
export type Standing = { latest: string; slots: number; back: number };

/**
 * The charts of a page as one: the layer their readouts are drawn on, over the page and outside
 * every card and strip, and whose readout is showing. One at a time: a chart that picks a day
 * takes the layer, and the one that had it lets its day go, whether it was picked by a pointer,
 * a tap or a key.
 *
 * And where they stand: scrolling one back through the days scrolls the rest to the same days.
 *
 * And the day picked: the others light the same day, without a readout of their own, so a day is
 * read across the page at once.
 */
type TimeGraph = {
  layer: HTMLElement | null;
  /** Shows a chart's readout, and drops the one that was showing. */
  claim: (chart: symbol, drop: () => void) => void;
  /** Says a chart's readout has gone. */
  release: (chart: symbol) => void;
  /** A chart scrolled by somebody: the others go to the same days, at the next frame. */
  scrolled: (chart: symbol, at: Standing) => void;
  /**
   * How far back the charts of these days stand, for one arriving among them or opening again:
   * none until one is scrolled. Where they stood among other days is forgotten, so coming back
   * to a range opens on its latest day, as it did the first time.
   */
  standing: (latest: string, slots: number) => number | null;
  /** Goes where the others are scrolled to, until the function it returns is called. */
  follow: (chart: symbol, go: (at: Standing) => void) => () => void;
  /**
   * The day a chart picked, or none. A chart that had none picked letting go says nothing: the
   * day is the last chart's to pick one, which the first lets go as it takes the readout.
   */
  light: (chart: symbol, day: string | null) => void;
  /**
   * Hears the day another chart picked, or none, until the function it returns is called: told
   * at once of one picked already.
   */
  lit: (chart: symbol, hear: (day: string | null) => void) => () => void;
};

const TimeGraphContext = createContext<TimeGraph | null>(null);

/** Once, at the root, for every chart under it. */
export function TimeGraphProvider({ children }: { children: ReactNode }) {
  const [layer, setLayer] = useState<HTMLElement | null>(null);
  const showing = useRef<{ chart: symbol; drop: () => void } | null>(null);
  const followers = useRef(new Map<symbol, (at: Standing) => void>());
  const stood = useRef<{ chart: symbol; at: Standing } | null>(null);
  const frame = useRef(0);
  const lighting = useRef<{ chart: symbol; day: string } | null>(null);
  const hearers = useRef(new Map<symbol, (day: string | null) => void>());
  const graph = useMemo<TimeGraph>(
    () => ({
      layer,
      claim(chart, drop) {
        const was = showing.current;
        showing.current = { chart, drop };
        if (was && was.chart !== chart) was.drop();
      },
      release(chart) {
        if (showing.current?.chart === chart) showing.current = null;
      },
      // A scroll is many events a frame; the others move once a frame, to the last of them.
      scrolled(chart, at) {
        stood.current = { chart, at };
        cancelAnimationFrame(frame.current);
        frame.current = requestAnimationFrame(() => {
          const now = stood.current;
          if (!now) return;
          for (const [other, go] of followers.current) {
            if (other !== now.chart) go(now.at);
          }
        });
      },
      standing(latest, slots) {
        const at = stood.current?.at;
        if (!at) return null;
        if (at.latest === latest && at.slots === slots) return at.back;
        stood.current = null;
        return null;
      },
      follow(chart, go) {
        followers.current.set(chart, go);
        return () => {
          followers.current.delete(chart);
        };
      },
      light(chart, day) {
        const now = lighting.current;
        if (day === null) {
          if (now?.chart !== chart) return;
          lighting.current = null;
        } else {
          if (now?.chart === chart && now.day === day) return;
          lighting.current = { chart, day };
        }
        // The chart that picked it has its own readout of it.
        for (const [other, hear] of hearers.current) {
          hear(other === chart ? null : day);
        }
      },
      lit(chart, hear) {
        hearers.current.set(chart, hear);
        const now = lighting.current;
        if (now && now.chart !== chart) hear(now.day);
        return () => {
          hearers.current.delete(chart);
        };
      },
    }),
    [layer],
  );
  return (
    <TimeGraphContext.Provider value={graph}>
      {children}
      <div ref={setLayer} />
    </TimeGraphContext.Provider>
  );
}

/** The page's charts. A chart is always on one, so one with no provider above it is a mistake. */
export function useTimeGraph(): TimeGraph {
  const graph = useContext(TimeGraphContext);
  if (!graph) throw new Error("a TimeChart needs a TimeGraphProvider above it");
  return graph;
}
