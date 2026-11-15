import { useState, type FormEvent } from "react";

import { postAuthLogin } from "@app/api/actions";
import { messageOf } from "@app/api/transport";
import { Button } from "@app/components/Button";
import { TextField } from "@app/components/TextField";
import { Wordmark } from "@app/components/Wordmark";
import { leaveFor } from "@app/leave";

/**
 * The only document a signed-out visitor loads: a few kilobytes instead of the whole
 * application. The username and password are the ones the server's environment names.
 */
export function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await postAuthLogin({ username, password });
      leaveFor("/");
    } catch (err) {
      setError(messageOf(err));
      setBusy(false);
    }
  };

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-sm flex-col justify-center gap-6 px-4">
      <h1 className="text-2xl font-semibold">
        <Wordmark />
      </h1>
      <form className="flex flex-col gap-3" onSubmit={submit}>
        <TextField
          name="username"
          autoComplete="username"
          placeholder="Username"
          className="w-full"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoFocus
        />
        <TextField
          name="password"
          type="password"
          autoComplete="current-password"
          placeholder="Password"
          className="w-full"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {error ? <p className="text-sm text-accent">{error}</p> : null}
        <Button
          type="submit"
          variant="solid"
          className="justify-center"
          disabled={busy}
        >
          Sign in
        </Button>
      </form>
    </main>
  );
}
