import { useState, useRef } from "react";
import { useMutation } from "@tanstack/react-query";
import { Terminal, Play, AlertCircle, CheckCircle } from "lucide-react";
import { ledgerClient } from "../client.js";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

const EXAMPLES = [
  { label: "Account balances", args: "bal" },
  { label: "Balance sheet (depth 2)", args: "bs --depth 2" },
  { label: "Income statement", args: "is --monthly" },
  { label: "Recent transactions", args: "print date:lastmonth.." },
  { label: "All accounts", args: "accounts --tree" },
  { label: "Tags in use", args: "tags" },
];

export function HledgerQueryPage() {
  const [args, setArgs] = useState("");
  const textareaRef = useRef(null);

  const { mutate, data, isPending, error, reset } = useMutation({
    mutationFn: (argsStr) => ledgerClient.runHledgerQuery({ args: argsStr }),
  });

  function run() {
    const trimmed = args.trim();
    if (!trimmed) return;
    mutate(trimmed);
  }

  function handleKeyDown(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      run();
    }
  }

  function applyExample(exArgs) {
    setArgs(exArgs);
    reset();
    textareaRef.current?.focus();
  }

  const hasResult = data != null;
  const stdout = data?.stdout ?? "";
  const stderr = data?.stderr ?? "";
  const success = data?.success ?? false;
  const cmdLine = data?.commandLine ?? "";

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="size-5" />
            hledger Query
          </CardTitle>
          <CardDescription>
            Run arbitrary hledger commands against your journal. Enter arguments only — do
            not include <code className="font-mono text-xs">hledger</code> or{" "}
            <code className="font-mono text-xs">-f &lt;journal&gt;</code>; the server adds
            those automatically.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="hledger-args">Arguments</Label>
            <div className="flex gap-2">
              <Textarea
                id="hledger-args"
                ref={textareaRef}
                value={args}
                onChange={(e) => setArgs(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="bal --depth 2 assets"
                className="font-mono resize-none"
                rows={2}
              />
              <Button
                onClick={run}
                disabled={isPending || !args.trim()}
                className="self-start shrink-0"
              >
                <Play className="size-4" />
                {isPending ? "Running…" : "Run"}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Press <kbd className="rounded border px-1 py-0.5 font-mono text-xs">Ctrl+Enter</kbd> to run.
            </p>
          </div>

          <div className="flex flex-wrap gap-2">
            <span className="text-xs text-muted-foreground self-center">Examples:</span>
            {EXAMPLES.map((ex) => (
              <Button
                key={ex.args}
                variant="outline"
                size="xs"
                className="font-mono text-xs"
                onClick={() => applyExample(ex.args)}
              >
                {ex.args}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertDescription>{error.message || String(error)}</AlertDescription>
        </Alert>
      )}

      {hasResult && (
        <Card>
          <CardHeader className="pb-2">
            <div className="flex items-center gap-2">
              {success ? (
                <CheckCircle className="size-4 text-green-500" />
              ) : (
                <AlertCircle className="size-4 text-destructive" />
              )}
              <CardTitle className="text-sm font-medium">
                {success ? "Success" : "Error"}
              </CardTitle>
            </div>
            <CardDescription className="font-mono text-xs break-all">
              $ {cmdLine}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {stdout && (
              <div className="flex flex-col gap-1">
                {stderr && <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">stdout</p>}
                <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs font-mono whitespace-pre">
                  {stdout}
                </pre>
              </div>
            )}
            {stderr && (
              <div className="flex flex-col gap-1">
                {stdout && <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">stderr</p>}
                <pre className="overflow-x-auto rounded-md bg-destructive/10 p-3 text-xs font-mono whitespace-pre text-destructive">
                  {stderr}
                </pre>
              </div>
            )}
            {!stdout && !stderr && (
              <p className="text-sm text-muted-foreground">(no output)</p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
