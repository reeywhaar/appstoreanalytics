import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AppleApp, HookEndpoint } from "@app/api/types";
import { AppsPanel } from "@app/islands/app/AppHooks";
import { mount } from "@app/test/harness";

const connect = vi.fn();

const endpoint = (
  id: number,
  label: string,
  app = "",
  part: HookEndpoint["part"] = "",
): HookEndpoint => ({
  id,
  label,
  url: `https://stats.example.com/hooks/${id}`,
  created_at: 0,
  count: 0,
  last_at: null,
  secret_set: true,
  app,
  part,
});
const endpoints = [
  endpoint(1, "Wengi: Webhook", "42", "webhook"),
  endpoint(2, "Builds"),
];

const apps: AppleApp[] = [
  {
    id: "42",
    name: "Wengi",
    webhook: { state: "off", url: endpoints[0]!.url, endpoint: 1 },
    production: {
      state: "unknown",
      url: "https://api.wengi.example.com/apple",
      endpoint: null,
    },
    sandbox: { state: "none", url: "", endpoint: null },
  },
];

vi.mock("@app/api/actions", () => ({
  getHooksApple: () => Promise.resolve({ apps }),
  postHooksAppleByAppConnect: (app: string, body: unknown) => {
    connect(app, body);
    return Promise.resolve({ result: "changed" });
  },
}));

afterEach(() => vi.clearAllMocks());

/** Opens the dialog of one of Wengi's parts, from the cell that says where it is sent. */
async function open(part: string) {
  fireEvent.click(
    await screen.findByRole("button", { name: new RegExp(`^Wengi, ${part}:`) }),
  );
  const dialog = await screen.findByRole("dialog");
  const segment = (name: string) =>
    within(dialog).getByRole("button", { name }) as HTMLButtonElement;
  const pressed = () =>
    ["Auto", "List", "Manual", "Empty"].find(
      (n) => segment(n).getAttribute("aria-pressed") === "true",
    );
  const save = () => fireEvent.click(segment("Save"));
  return { dialog, segment, pressed, save };
}

describe("an app's parts", () => {
  it("say where each is sent: an address, a warning, or nothing", async () => {
    mount(<AppsPanel endpoints={endpoints} />);
    for (const name of [
      "Wengi, Webhook: Webhook, turned off",
      "Wengi, Production: Unknown: api.wengi.example.com",
      "Wengi, Sandbox: Empty",
    ]) {
      expect(await screen.findByRole("button", { name })).toBeDefined();
    }
  });

  it("are changed to one of the list, saying what an unknown one replaced", async () => {
    mount(<AppsPanel endpoints={endpoints} />);
    const { dialog, segment, pressed, save } = await open("Production");
    // Opened on what it is: typed in, as none of the addresses is it.
    expect(pressed()).toBe("Manual");
    expect(dialog.textContent).not.toContain("stops hearing");

    fireEvent.click(segment("List"));
    fireEvent.change(
      within(dialog).getByRole("combobox", { name: "Address" }),
      {
        target: { value: "2" },
      },
    );
    expect(dialog.textContent).toContain(
      "If that is Wengi’s own server, it stops hearing of its subscriptions.",
    );
    save();
    await vi.waitFor(() =>
      expect(connect).toHaveBeenCalledWith("42", {
        part: "production",
        endpoint: 2,
      }),
    );
  });

  it("are connected automatically, an empty one by default, to the app's main address", async () => {
    mount(<AppsPanel endpoints={endpoints} />);
    const { dialog, segment, pressed, save } = await open("Sandbox");
    expect(pressed()).toBe("Auto");
    expect(segment("Main").getAttribute("aria-pressed")).toBe("true");
    expect(dialog.textContent).toContain("added to the list above as “Wengi”");
    fireEvent.click(segment("Separate"));
    expect(dialog.textContent).toContain(
      "added to the list above as “Wengi: Sandbox”",
    );
    save();
    await vi.waitFor(() =>
      expect(connect).toHaveBeenCalledWith("42", {
        part: "sandbox",
        auto: "separate",
      }),
    );
  });

  it("are a webhook typed in only with its secret, and Empty deletes it", async () => {
    mount(<AppsPanel endpoints={endpoints} />);
    const { dialog, segment, pressed, save } = await open("Webhook");
    // Its own separate automatic address, turned off: it opens there, which sets it right.
    expect(pressed()).toBe("Auto");
    expect(segment("Separate").getAttribute("aria-pressed")).toBe("true");
    expect(dialog.textContent).toContain("It is “Wengi: Webhook”");
    fireEvent.click(segment("Manual"));
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Address" }), {
      target: { value: "https://ci.example.com/hook" },
    });
    expect(segment("Save").disabled).toBe(true);
    fireEvent.change(within(dialog).getByRole("textbox", { name: /Secret/ }), {
      target: { value: "theirs" },
    });
    expect(segment("Save").disabled).toBe(false);

    fireEvent.click(segment("Empty"));
    expect(dialog.textContent).toContain("The webhook is deleted");
    save();
    await vi.waitFor(() =>
      expect(connect).toHaveBeenCalledWith("42", { part: "webhook" }),
    );
  });
});
