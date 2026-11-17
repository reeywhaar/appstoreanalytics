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
    fireEvent.pointerMove(hit, { clientX: 10 });
    expect(screen.queryByText("2 downloads")).toBeNull();
    // The readout says the value alone; a screen reader's line says the day before it.
    expect(screen.getByText(/^\d+ updates$/)).toBeDefined();
  });
});
