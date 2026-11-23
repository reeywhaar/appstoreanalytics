import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { TimeChart } from "@app/charts/TimeChart";
import { mount } from "@app/test/harness";

const days = ["2026-11-14", "2026-11-15"];

describe("TimeChart", () => {
  /**
   * A keyboard can leave one chart's day picked while a pointer picks another's: the page shows
   * one readout, the last chart's, and the first lets its day go.
   */
  it("shows one readout on the page at a time", () => {
    mount(
      <>
        <TimeChart
          label="Downloads"
          days={days}
          values={[1, 2]}
          format={(n) => `${n} downloads`}
          axis={String}
        />
        <TimeChart
          label="Updates"
          days={days}
          values={[10, 20]}
          format={(n) => `${n} updates`}
          axis={String}
        />
      </>,
    );
    const downloads = screen.getByRole("group", { name: /^Downloads/ });
    const updates = screen.getByRole("group", { name: /^Updates/ });

    fireEvent.focus(downloads);
    expect(screen.getByText("2 downloads")).toBeDefined();

    const hit = updates.querySelector('rect[fill="transparent"]')!;
    fireEvent.pointerMove(hit, { pointerType: "mouse", clientX: 10 });
    expect(screen.queryByText("2 downloads")).toBeNull();
    // The readout says the value alone; a screen reader's line says the day before it.
    expect(screen.getByText(/^\d+ updates$/)).toBeDefined();
  });

  /**
   * The day picked on one chart is lit on the others, the rest of their days dimmed, with the
   * readout only where it was picked; and let go everywhere once the pointer leaves.
   */
  it("lights the day picked on one chart on the others", () => {
    mount(
      <>
        <TimeChart
          label="Downloads"
          days={days}
          values={[1, 2]}
          format={(n) => `${n} downloads`}
          axis={String}
        />
        <TimeChart
          label="Updates"
          days={days}
          values={[10, 20]}
          format={(n) => `${n} updates`}
          axis={String}
        />
      </>,
    );
    const downloads = screen.getByRole("group", { name: /^Downloads/ });
    const updates = screen.getByRole("group", { name: /^Updates/ });
    const columns = () =>
      [...downloads.querySelectorAll("g.transition-opacity")].map((g) =>
        g.getAttribute("opacity"),
      );
    expect(columns()).toEqual(["1", "1"]);

    const hit = updates.querySelector('rect[fill="transparent"]')!;
    fireEvent.pointerMove(hit, { pointerType: "mouse", clientX: 10 });
    // One day lit, the one the readout on Updates says, and no readout on Downloads.
    const lit = columns().indexOf("1");
    expect(columns().filter((o) => o === "1")).toHaveLength(1);
    expect(updates.textContent).toContain(`${days[lit]}: `);
    expect(screen.queryByText(/^\d+ downloads$/)).toBeNull();

    fireEvent.pointerLeave(hit, { pointerType: "mouse" });
    expect(columns()).toEqual(["1", "1"]);
  });

  /**
   * A finger put on a chart to scroll the page is not a tap: nothing is picked when the browser
   * takes the press for a scroll, nor by the chart taking focus from it. A tap picks its day.
   */
  it("picks a day on a tap, and not on a scroll that starts on it", () => {
    mount(
      <TimeChart
        label="Downloads"
        days={days}
        values={[1, 2]}
        format={(n) => `${n} downloads`}
        axis={String}
      />,
    );
    const chart = screen.getByRole("group", { name: /^Downloads/ });
    const hit = chart.querySelector('rect[fill="transparent"]')!;
    const touch = { pointerType: "touch", clientX: 600, clientY: 50 };

    // A scroll: down, the focus a press brings, and the browser cancelling the press.
    fireEvent.pointerDown(hit, touch);
    fireEvent.focus(chart);
    fireEvent.pointerCancel(hit, touch);
    expect(screen.queryByText(/^\d+ downloads$/)).toBeNull();

    // A tap: down and up where it came down.
    fireEvent.pointerDown(hit, touch);
    fireEvent.pointerUp(hit, touch);
    expect(screen.getByText(/^\d+ downloads$/)).toBeDefined();
  });
});

/** Apps a, b, c… with one day each: the values given, in the series' order. */
function apps(values: number[]) {
  return values.map((v, i) => {
    const id = String.fromCharCode(97 + i);
    return { id, name: `App ${id}`, color: "red", values: [v] };
  });
}

/** The day's chart, picked as the keyboard picks it, and the readout's lines. */
function picked(values: number[]) {
  mount(
    <TimeChart
      label="Downloads"
      days={["2026-11-15"]}
      values={[values.reduce((t, v) => t + v, 0)]}
      series={apps(values)}
      format={(n) => `${n} downloads`}
      cell={String}
      axis={String}
    />,
  );
  const chart = screen.getByRole("group", { name: /^Downloads/ });
  fireEvent.focus(chart);
  const lines = [...document.querySelectorAll("li")].map((li) =>
    li.textContent!.trim(),
  );
  return { chart, lines };
}

describe("a day's readout of its apps", () => {
  it("lists those with anything, the largest first", () => {
    const { lines } = picked([0, 3, 0, 9, 1]);
    expect(lines).toEqual(["9App d", "3App b", "1App e"]);
  });

  it("names every one when there is one more than seven", () => {
    expect(picked([8, 7, 6, 5, 4, 3, 2, 1]).lines).toHaveLength(8);
  });

  it("names seven and sums up the rest past one more", () => {
    const { lines } = picked([9, 8, 7, 6, 5, 4, 3, 2, 1]);
    expect(lines.slice(0, 7)).toEqual([
      "9App a",
      "8App b",
      "7App c",
      "6App d",
      "5App e",
      "4App f",
      "3App g",
    ]);
    expect(lines[7]).toBe("32 others");
    expect(screen.getByText("Enter for every app")).toBeDefined();
  });

  it("shows every app of the day in a dialog on Enter", () => {
    const { chart } = picked([9, 8, 7, 6, 5, 4, 3, 2, 1]);
    fireEvent.keyDown(chart, { key: "Enter" });
    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent).toContain("Sun 2026-11-15");
    expect(dialog.textContent).toContain("45 downloads");
    expect(dialog.querySelectorAll("tbody tr")).toHaveLength(9);
  });
});
