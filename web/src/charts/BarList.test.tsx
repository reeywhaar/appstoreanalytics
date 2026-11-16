import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { BarList } from "@app/charts/BarList";

const rows = [
  { id: "us", name: "United States", text: "United States", values: [44, 1] },
  { id: "de", name: "Germany", text: "Germany", values: [25, 4.24] },
  { id: "fr", name: "France", text: "France", values: [5, 2] },
];

const columns = [
  { label: "Downloads", format: String },
  { label: "Proceeds", format: String },
];

/** The names of the rows, top to bottom. */
const order = () =>
  screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[0]!.textContent);

describe("a ranked list", () => {
  it("keeps the order it was given until a header is pressed", () => {
    render(
      <BarList
        label="By country"
        named="Country"
        columns={columns}
        rows={rows}
      />,
    );
    expect(order()).toEqual(["United States", "Germany", "France"]);
  });

  it("sorts by a column, largest first, and turns round when pressed again", () => {
    render(
      <BarList
        label="By country"
        named="Country"
        columns={columns}
        rows={rows}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Proceeds/ }));
    expect(order()).toEqual(["Germany", "France", "United States"]);
    expect(
      screen
        .getByRole("columnheader", { name: /Proceeds/ })
        .getAttribute("aria-sort"),
    ).toBe("descending");
    fireEvent.click(screen.getByRole("button", { name: /Proceeds/ }));
    expect(order()).toEqual(["United States", "France", "Germany"]);
  });

  it("sorts by name from A", () => {
    render(
      <BarList
        label="By country"
        named="Country"
        columns={columns}
        rows={rows}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Country/ }));
    expect(order()).toEqual(["France", "Germany", "United States"]);
  });
});
