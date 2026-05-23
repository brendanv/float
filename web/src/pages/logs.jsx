import { useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { LedgerService } from "@/gen/float/v1/ledger_pb.js";
import { Page, PageCard } from "../components/page.jsx";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ScrollText, Trash2, CircleAlert } from "lucide-react";
import { PageHeader } from "../components/page-header.jsx";

const LEVELS = ["DEBUG", "INFO", "WARN", "ERROR"];

const logsClient = createClient(LedgerService, createGrpcWebTransport({
  baseUrl: window.location.origin,
}));

function levelVariant(level) {
  if (level === "ERROR") return "destructive";
  if (level === "WARN") return "secondary";
  return "outline";
}

function formatAttrs(attrs) {
  const entries = Object.entries(attrs ?? {});
  if (entries.length === 0) return "";
  return entries
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join(" ");
}

export function LogsPage() {
  const [minLevel, setMinLevel] = useState("INFO");
  const [entries, setEntries] = useState([]);
  const [streamError, setStreamError] = useState(null);
  const [status, setStatus] = useState("connecting");
  const listRef = useRef(null);
  const demoMode = typeof window !== "undefined" && window.location.hash.includes("demoLogs=1");

  useEffect(() => {
    if (demoMode) {
      setEntries([
        { time: "2026-05-16T12:01:00.000Z", level: "INFO", message: "floatd started", attrs: { addr: ":8080" } },
        { time: "2026-05-16T12:01:02.000Z", level: "WARN", message: "slow query", attrs: { duration_ms: "1220", method: "ListTransactions" } },
      ]);
      setStatus("connected");
      return;
    }
    const abortController = new AbortController();
    let cancelled = false;

    async function consume() {
      setStatus("connecting");
      setStreamError(null);
      try {
        for await (const res of logsClient.streamLogs(
          { minLevel },
          { signal: abortController.signal }
        )) {
          if (cancelled) return;
          const entry = res.entry;
          if (!entry) continue;
          setEntries((prev) => [...prev.slice(-399), entry]);
          setStatus("connected");
        }
      } catch (err) {
        if (abortController.signal.aborted || cancelled) return;
        setStreamError(err);
        setStatus("error");
      }
    }

    consume();

    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [demoMode, minLevel]);

  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight });
  }, [entries]);

  const statusBadge = useMemo(() => {
    if (status === "connected") return <Badge variant="outline">Live</Badge>;
    if (status === "connecting") return <Badge variant="secondary">Connecting...</Badge>;
    return <Badge variant="destructive">Disconnected</Badge>;
  }, [status]);

  return (
    <Page>
      <PageHeader
        title="Logs"
        description="Live server logs via gRPC stream. The connection closes automatically when you leave this page."
      />

      <PageCard
        title={
          <span className="flex items-center gap-2">
            <ScrollText />
            Stream
          </span>
        }
        description="Showing up to the latest 400 log entries."
        action={statusBadge}
        contentClassName="flex flex-col gap-3"
      >
        <div className="flex flex-wrap items-center gap-2">
          <Select value={minLevel} onValueChange={setMinLevel}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Minimum level" />
            </SelectTrigger>
            <SelectContent>
              {LEVELS.map((level) => (
                <SelectItem key={level} value={level}>{level}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="outline" size="sm" onClick={() => setEntries([])}>
            <Trash2 data-icon="inline-start" />
            Clear
          </Button>
        </div>

        {streamError && (
          <Alert variant="destructive">
            <CircleAlert />
            <AlertDescription>{streamError.message || String(streamError)}</AlertDescription>
          </Alert>
        )}

        <div ref={listRef} className="h-[60vh] overflow-auto rounded-md border bg-muted/20 p-2 font-mono text-xs">
          {entries.length === 0 ? (
            <p className="px-2 py-1 text-muted-foreground">Waiting for log entries...</p>
          ) : (
            <ul className="flex flex-col gap-1">
              {entries.map((entry, idx) => (
                <li key={`${entry.time}-${idx}`} className="rounded border bg-background px-2 py-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-muted-foreground">{entry.time}</span>
                    <Badge variant={levelVariant(entry.level)} className="font-mono">{entry.level}</Badge>
                    <span className="break-all">{entry.message}</span>
                  </div>
                  {formatAttrs(entry.attrs) && (
                    <div className="mt-1 text-muted-foreground break-all">{formatAttrs(entry.attrs)}</div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </PageCard>
    </Page>
  );
}
