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
