import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  TimeGraphProvider,
  useTimeGraph,
  type Standing,
} from "@app/charts/TimeGraph";

/** The page's charts, as a chart sees them. */
function graphOf() {
  let graph: ReturnType<typeof useTimeGraph> | null = null;
  function Grab() {
    graph = useTimeGraph();
    return null;
  }
  render(
    <TimeGraphProvider>
      <Grab />
    </TimeGraphProvider>,
  );
  return graph!;
}

const frame = () =>
  new Promise<void>((done) => requestAnimationFrame(() => done()));

const back = (days: number): Standing => ({
  latest: "2026-11-15",
  slots: 30,
  back: days,
});

describe("the page's charts scrolled as one", () => {
  it("takes the others where one is scrolled, at the next frame, and not the one itself", async () => {
    const graph = graphOf();
    const a = Symbol("a");
    const b = Symbol("b");
    const toA = vi.fn();
    const toB = vi.fn();
    graph.follow(a, toA);
    graph.follow(b, toB);

    graph.scrolled(a, back(3));
    graph.scrolled(a, back(5));
    expect(toB).not.toHaveBeenCalled();
    await frame();
    // Once a frame, to the last of a scroll's many events.
    expect(toB).toHaveBeenCalledTimes(1);
    expect(toB).toHaveBeenCalledWith(back(5));
    expect(toA).not.toHaveBeenCalled();
  });

  it("stops taking a chart along once it stops following", async () => {
    const graph = graphOf();
    const a = Symbol("a");
    const b = Symbol("b");
    const toB = vi.fn();
    const stop = graph.follow(b, toB);
    stop();
    graph.scrolled(a, back(4));
    await frame();
    expect(toB).not.toHaveBeenCalled();
  });

  it("tells a chart arriving where the others stand among the same days", () => {
    const graph = graphOf();
    expect(graph.standing("2026-11-15", 30)).toBeNull();
    graph.scrolled(Symbol("a"), back(12));
    expect(graph.standing("2026-11-15", 30)).toBe(12);
  });

  it("forgets where they stood among other days, so a range comes back on its latest day", () => {
    const graph = graphOf();
    graph.scrolled(Symbol("a"), back(12));
    // Another range: nothing of it, and the first is forgotten.
    expect(graph.standing("2026-11-15", 90)).toBeNull();
    expect(graph.standing("2026-11-15", 30)).toBeNull();
  });
});

describe("the day picked, lit on every chart", () => {
  it("tells the others the day a chart picked, and the chart itself none", () => {
    const graph = graphOf();
    const a = Symbol("a");
    const b = Symbol("b");
    const atA = vi.fn();
    const atB = vi.fn();
    graph.lit(a, atA);
    graph.lit(b, atB);

    graph.light(a, "2026-11-14");
    expect(atB).toHaveBeenLastCalledWith("2026-11-14");
    expect(atA).toHaveBeenLastCalledWith(null);

    graph.light(a, null);
    expect(atB).toHaveBeenLastCalledWith(null);
  });

  it("keeps the last chart's day when the one before lets its own go", () => {
    const graph = graphOf();
    const a = Symbol("a");
    const b = Symbol("b");
    const atA = vi.fn();
    graph.lit(a, atA);

    graph.light(a, "2026-11-14");
    graph.light(b, "2026-11-15");
    // a lets go as b takes the readout, after b has picked.
    graph.light(a, null);
    expect(atA).toHaveBeenLastCalledWith("2026-11-15");
  });

  it("tells a chart arriving the day already picked, and stops telling one that went", () => {
    const graph = graphOf();
    const a = Symbol("a");
    graph.light(a, "2026-11-14");
    const atB = vi.fn();
    const stop = graph.lit(Symbol("b"), atB);
    expect(atB).toHaveBeenCalledWith("2026-11-14");
    stop();
    graph.light(a, "2026-11-15");
    expect(atB).toHaveBeenCalledTimes(1);
  });
});
