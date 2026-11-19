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

/** A row with two apps' parts in it, figure by figure as the columns are. */
const shared = {
  id: "us",
  name: "United States",
  text: "United States",
  values: [8, 3],
  parts: [
    { id: "1", name: "Dots", color: "red", values: [5, 3] },
    { id: "2", name: "Lines", color: "blue", values: [3, 0] },
  ],
};

describe("a ranked list told apart by app", () => {
  it("draws each app's part of a bar in its colour, along the column sorted by", () => {
    const { container } = render(
      <BarList label="By country" columns={columns} rows={[shared]} />,
    );
    const segments = [
      ...container.querySelectorAll<HTMLElement>("[style*='flex']"),
    ];
    expect(segments.map((s) => s.style.backgroundColor)).toEqual([
      "red",
      "blue",
    ]);

    // Sorted by proceeds, Lines had none of them, and has no part of the bar.
    fireEvent.click(screen.getByRole("button", { name: /Proceeds/ }));
    const now = [...container.querySelectorAll<HTMLElement>("[style*='flex']")];
    expect(now.map((s) => s.style.backgroundColor)).toEqual(["red"]);
  });

  it("opens a row's apps, ranked, with its figures", () => {
    render(
      <BarList
        label="By country"
        named="Country"
        columns={columns}
        rows={[shared]}
        detail
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "United States: each app" }),
    );
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("United States")).toBeDefined();
    const apps = within(dialog)
      .getAllByRole("row")
      .slice(1)
      .map((row) => within(row).getAllByRole("cell")[0]!.textContent);
    expect(apps).toEqual(["Dots", "Lines"]);
  });

  it("opens nothing without detail", () => {
    render(<BarList label="By country" columns={columns} rows={[shared]} />);
    expect(screen.queryByRole("button", { name: /each app/ })).toBeNull();
  });
});
