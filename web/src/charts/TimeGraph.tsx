import {
  createContext,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

/**
 * The charts of a page as one: the layer their readouts are drawn on, over the page and outside
 * every card and strip, and whose readout is showing. One at a time: a chart that picks a day
 * takes the layer, and the one that had it lets its day go, whether it was picked by a pointer,
 * a tap or a key.
 */
type TimeGraph = {
  layer: HTMLElement | null;
  /** Shows a chart's readout, and drops the one that was showing. */
  claim: (chart: symbol, drop: () => void) => void;
  /** Says a chart's readout has gone. */
  release: (chart: symbol) => void;
};

const TimeGraphContext = createContext<TimeGraph | null>(null);

/** Once, at the root, for every chart under it. */
export function TimeGraphProvider({ children }: { children: ReactNode }) {
  const [layer, setLayer] = useState<HTMLElement | null>(null);
  const showing = useRef<{ chart: symbol; drop: () => void } | null>(null);
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
