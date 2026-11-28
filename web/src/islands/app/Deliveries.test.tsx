import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Delivery } from "@app/api/types";
import { Deliveries } from "@app/islands/app/Deliveries";
import { mount } from "@app/test/harness";

const delivery: Delivery = {
  id: 7,
  received_at: 1795400000,
  via: "Wallape",
  event: "TEST",
  summary: "TEST · com.example.wallape · Production",
  size: 1200,
  proven_by: "apple",
  proof_error: "",
  mailed_at: null,
  mailed_to: [],
  mail_error: "",
};

vi.mock("@app/api/actions", () => ({
  getHooksDeliveries: () => Promise.resolve({ deliveries: [delivery] }),
  getHooksById: () =>
    Promise.resolve({
      ...delivery,
      content_type: "application/json",
      body: "{}",
      decoded: {},
    }),
}));

describe("a delivery", () => {
  it("opens from a press anywhere on its line", async () => {
    mount(<Deliveries />);
    const line = await screen.findByRole("button", {
      name: /TEST · com\.example\.wallape · Production/,
    });
    expect(within(line).getByText("signed by Apple")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Info" })).toBeNull();
    fireEvent.click(line);
    expect(await screen.findByRole("dialog")).toBeDefined();
  });
});
