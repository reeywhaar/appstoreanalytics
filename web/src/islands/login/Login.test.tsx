import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@app/api/transport";
import { Login } from "@app/islands/login/Login";

const postAuthLogin = vi.fn();
const leaveFor = vi.fn();

vi.mock("@app/api/actions", () => ({
  postAuthLogin: (body: unknown) => postAuthLogin(body),
}));
vi.mock("@app/leave", () => ({ leaveFor: (path: string) => leaveFor(path) }));

afterEach(() => vi.clearAllMocks());

const type = (placeholder: string, value: string) =>
  fireEvent.change(screen.getByPlaceholderText(placeholder), {
    target: { value },
  });

describe("signing in", () => {
  it("goes to the dashboard once the password matches", async () => {
    postAuthLogin.mockResolvedValue(undefined);
    render(<Login />);
    type("Username", "misha");
    type("Password", "a good password");
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(leaveFor).toHaveBeenCalledWith("/"));
    expect(postAuthLogin).toHaveBeenCalledWith({
      username: "misha",
      password: "a good password",
    });
  });

  // The server writes the sentence for whoever reads it.
  it("says what the server said when it does not", async () => {
    postAuthLogin.mockRejectedValue(
      new ApiError(
        401,
        "unauthenticated",
        "That username and password do not match.",
      ),
    );
    render(<Login />);
    type("Username", "misha");
    type("Password", "not it");
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(
      await screen.findByText("That username and password do not match."),
    ).toBeTruthy();
    expect(leaveFor).not.toHaveBeenCalled();
  });
});
