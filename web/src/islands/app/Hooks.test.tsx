import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Hooks, Relay } from "@app/api/types";
import { HooksPanels } from "@app/islands/app/Hooks";
import { mount } from "@app/test/harness";

const postHooksMails = vi.fn();

const endpoint = (id: number, label: string) => ({
  id,
  label,
  url: `https://example.com/hooks/${label}`,
  created_at: 0,
  count: 0,
  last_at: null,
  secret_set: true,
});

const hooks: Hooks = {
  endpoints: [endpoint(1, "Wengi"), endpoint(2, "Balo"), endpoint(3, "Roobe")],
  mails: [],
  events: [],
  catalog: [
    {
      name: "App Store",
      groups: [
        {
          name: "Subscriptions",
          kinds: [{ name: "SUBSCRIBED", title: "Subscribed", meaning: "" }],
        },
      ],
    },
  ],
};

const relay: Relay = {
  configured: true,
  host: "",
  port: 587,
  security: "starttls",
  username: "",
  password_set: false,
  from_address: "",
  from_name: "",
};

vi.mock("@app/api/actions", () => ({
  getHooks: () => Promise.resolve(hooks),
  getSettingsRelay: () => Promise.resolve(relay),
  postHooksMails: (body: unknown) => postHooksMails(body),
}));

afterEach(() => vi.clearAllMocks());

/** The dialog for a new recipient, and its boxes for the addresses. */
async function adding() {
  mount(<HooksPanels />);
  fireEvent.click(
    await screen.findByRole("button", { name: "Add a recipient…" }),
  );
  const dialog = await screen.findByRole("dialog");
  const group = within(dialog).getByRole("group", { name: "Addresses" });
  const box = (label: string | RegExp) =>
    within(group).getByRole("checkbox", {
      name: typeof label === "string" ? new RegExp(label) : label,
    }) as HTMLInputElement;
  const ticked = () => ["Wengi", "Balo", "Roobe"].filter((l) => box(l).checked);
  const every = () => box(/Every address/);
  /** The box over the addresses, as what it shows, and the addresses ticked. */
  const state = () => {
    const over = every().indeterminate
      ? "dash"
      : every().checked
        ? "ticked"
        : "empty";
    return { over, ticked: ticked() };
  };
  /** What the recipient is saved with, once they have an address and a kind. */
  const saving = async () => {
    fireEvent.change(within(dialog).getByPlaceholderText("you@example.com"), {
      target: { value: "me@example.com" },
    });
    fireEvent.click(
      within(dialog).getByRole("checkbox", { name: /SUBSCRIBED/ }),
    );
    const add = screen.getByRole("button", {
      name: "Add",
    }) as HTMLButtonElement;
    if (add.disabled) return "not saved";
    fireEvent.click(add);
    await waitFor(() => expect(postHooksMails).toHaveBeenCalled());
    return (postHooksMails.mock.calls[0]![0] as { endpoints: number[] })
      .endpoints;
  };
  return { box, every, state, saving };
}

const everyOne = ["Wengi", "Balo", "Roobe"];

describe("the addresses somebody is mailed from", () => {
  it("are every one, those added later too, unless chosen otherwise", async () => {
    postHooksMails.mockResolvedValue(undefined);
    const { state, saving } = await adding();
    expect(state()).toEqual({ over: "ticked", ticked: everyOne });
    expect(await saving()).toEqual([]);
  });

  it("are none from the box over them, which cannot be saved", async () => {
    const { every, state, saving } = await adding();
    fireEvent.click(every());
    expect(state()).toEqual({ over: "empty", ticked: [] });
    expect(
      screen.getByText("Choose at least one address to be mailed from."),
    ).toBeTruthy();
    expect(await saving()).toBe("not saved");
  });

  it("are every one again from the box over them, after none", async () => {
    const { every, state } = await adding();
    fireEvent.click(every());
    fireEvent.click(every());
    expect(state()).toEqual({ over: "ticked", ticked: everyOne });
  });

  it("are some, once one is let go", async () => {
    postHooksMails.mockResolvedValue(undefined);
    const { box, state, saving } = await adding();
    fireEvent.click(box("Wengi"));
    expect(state()).toEqual({ over: "dash", ticked: ["Balo", "Roobe"] });
    expect(await saving()).toEqual([2, 3]);
  });

  it("stay none once each is let go, rather than coming back as every one", async () => {
    const { box, state } = await adding();
    for (const l of everyOne) fireEvent.click(box(l));
    expect(state()).toEqual({ over: "empty", ticked: [] });
  });

  it("are all there are now once each is picked, not those added later", async () => {
    postHooksMails.mockResolvedValue(undefined);
    const { box, every, state, saving } = await adding();
    fireEvent.click(every());
    for (const l of everyOne) fireEvent.click(box(l));
    expect(state()).toEqual({ over: "dash", ticked: everyOne });
    expect(await saving()).toEqual([1, 2, 3]);
  });

  it("are every one from the box over them, after some", async () => {
    const { box, every, state } = await adding();
    fireEvent.click(box("Balo"));
    fireEvent.click(every());
    expect(state()).toEqual({ over: "ticked", ticked: everyOne });
  });
});
