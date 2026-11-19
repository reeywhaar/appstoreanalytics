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
