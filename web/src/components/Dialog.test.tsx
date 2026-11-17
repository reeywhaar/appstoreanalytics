import { useEffect, useState } from "react";
import { screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Dialog } from "@app/components/Dialog";
import { mount } from "@app/test/harness";

function Breaks(): never {
  throw new Error("the data was not what it claimed");
}

/** A body that is something else as the dialog opens, and then a field asking for focus. */
function Swapped() {
  const [ready, setReady] = useState(false);
  useEffect(() => setReady(true), []);
  return ready ? (
    <input data-autofocus aria-label="Label" />
  ) : (
    <p>What was added last time</p>
  );
}

/** A field that, once gone to, brings another asking for focus. */
function Late() {
  const [ready, setReady] = useState(false);
  return (
    <>
      <input
        aria-label="Secret"
        onFocus={() => setReady(true)}
        data-testid="secret"
      />
      {ready ? <input data-autofocus aria-label="Label" /> : null}
    </>
  );
}

describe("Dialog", () => {
  /**
   * showModal focuses the first control it finds whether or not that control wanted it, and in
   * the task editor that is Delete — a ring on a destructive action reads as armed.
   *
   * jsdom's showModal is the stand-in in test/setup.ts and steals nothing, so what this pins is
   * where focus ends up rather than the taking back. A browser was what showed the ring.
   */
  it("leaves nothing lit when no field asked for focus", () => {
    mount(
      <Dialog
        open
        onClose={vi.fn()}
        title="Task"
        footer={<button type="button">Delete</button>}
      >
        <input aria-label="Title" />
      </Dialog>,
    );
    expect(document.activeElement?.tagName).toBe("DIALOG");
  });

  /**
   * The title stays put and the body scrolls under it. It used to be the first thing inside the
   * scroller, so a long description scrolled its own name off the top and left a page of prose
   * with nothing saying what it belonged to.
   */
  it("keeps the title out of the part that scrolls", () => {
    const { container } = mount(
      <Dialog open onClose={vi.fn()} title="Task">
        <p>Something long</p>
      </Dialog>,
    );
    const scroller = container.querySelector(".overflow-y-auto")!;
    const heading = screen.getByRole("heading", { name: "Task" });
    expect(scroller).not.toBeNull();
    expect(scroller.contains(heading)).toBe(false);
    expect(scroller.textContent).toBe("Something long");
  });

  /**
   * What the title is about goes on the title's line. On a row of its own underneath it cost a
   * row and a gap off the top of every task, which on a phone is a twentieth of the screen
   * before the writing starts.
   */
  it("puts the aside beside the title, out of the scrolling part", () => {
    const { container } = mount(
      <Dialog open onClose={vi.fn()} title="Task" aside={<span>8qw4tz9k</span>}>
        <p>Something</p>
      </Dialog>,
    );
    const id = screen.getByText("8qw4tz9k");
    const heading = screen.getByRole("heading", { name: "Task" });
    expect(heading.parentElement!.contains(id)).toBe(true);
    expect(container.querySelector(".overflow-y-auto")!.contains(id)).toBe(
      false,
    );
  });

  // The task dialog's: its project, id and age already say what it is.
  it("keeps a hidden title for a screen reader", () => {
    mount(
      <Dialog open onClose={vi.fn()} title="Task" hideTitle>
        <p>Something</p>
      </Dialog>,
    );
    expect(screen.getByRole("heading", { name: "Task" }).className).toBe(
      "sr-only",
    );
  });

  // data-autofocus, because React's autoFocus is a call that runs while this is still hidden.
  it("gives focus back to a field that asked for it", () => {
    mount(
      <Dialog
        open
        onClose={vi.fn()}
        title="Change your password"
        footer={<button type="button">Cancel</button>}
      >
        <input data-autofocus aria-label="Current password" />
      </Dialog>,
    );
    expect(document.activeElement).toBe(
      screen.getByLabelText("Current password"),
    );
  });

  /**
   * A body swapped as it opens, as a dialog that shows a result and is opened again for another
   * does: the field that asks for focus comes after the opening, and still gets it.
   */
  it("gives focus to a field that comes after it opened", async () => {
    mount(
      <Dialog open onClose={vi.fn()} title="Add an address">
        <Swapped />
      </Dialog>,
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByLabelText("Label")),
    );
  });

  /** A field somebody has gone to keeps focus when another asking for it comes. */
  it("leaves focus where somebody put it", async () => {
    mount(
      <Dialog open onClose={vi.fn()} title="Add an address">
        <Late />
      </Dialog>,
    );
    const secret = screen.getByLabelText("Secret");
    secret.focus();
    await screen.findByLabelText("Label");
    await new Promise((r) => setTimeout(r, 0));
    expect(document.activeElement).toBe(secret);
  });

  /** A body that throws is one dialog's trouble, and its close still closes it. */
  it("keeps its title and close when the body fails", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const onClose = vi.fn();
    mount(
      <Dialog open onClose={onClose} title="Task">
        <Breaks />
      </Dialog>,
    );
    expect(screen.getByText(/could not be shown/)).toBeDefined();
    screen.getByRole("button", { name: "Close" }).click();
    expect(onClose).toHaveBeenCalled();
  });
});
