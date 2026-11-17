import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginPage() {
  const queryClient = useQueryClient();
  const [passphrase, setPassphrase] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // If auth is disabled on the server there is nothing to log into.
  useEffect(() => {
    fetch("/api/auth")
      .then((res) => res.json())
      .then(({ enabled }) => {
        if (!enabled) window.location.hash = "#/";
      })
      .catch(() => {});
  }, []);

  async function handleSubmit(e) {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ passphrase }),
      });
      if (res.status === 401) {
        setError("Incorrect passphrase.");
        return;
      }
      if (!res.ok) {
        setError(`Login failed (${res.status}).`);
        return;
      }
      // The session cookie is now set; drop any cached failures and reload data.
      queryClient.clear();
      window.location.hash = "#/";
    } catch {
      setError("Could not reach the server.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex min-h-svh items-center justify-center bg-background px-4 py-12">
      <div className="flex w-full max-w-xs flex-col items-center gap-8">
        <h1 className="inline-flex items-center gap-2 text-[34px] leading-none font-semibold tracking-[-0.04em] text-primary">
          float
          <i aria-hidden="true" className="inline-block size-2.5 translate-y-1 rounded-full bg-primary" />
        </h1>
        <form onSubmit={handleSubmit} className="flex w-full flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="passphrase" className="text-control">Passphrase</Label>
            <Input
              id="passphrase"
              type="password"
              autoFocus
              autoComplete="current-password"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "login-error" : undefined}
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              className="h-9"
            />
          </div>
          {error && <p id="login-error" role="alert" className="text-control text-below">{error}</p>}
          <Button type="submit" size="lg" disabled={submitting || !passphrase}>
            {submitting ? "Signing in…" : "Sign in"}
          </Button>
        </form>
      </div>
    </main>
  );
}
