import { useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { LedgerService } from "@/gen/float/v1/ledger_pb.js";
import { Page, Section } from "../components/page.jsx";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/alert";
import { cn } from "@/lib/utils";

const LEVELS = ["DEBUG", "INFO", "WARN", "ERROR"];

const logsClient = createClient(LedgerService, createGrpcWebTransport({
  baseUrl: window.location.origin,
}));

// Log levels are color-coded: errors in below, warnings in warning, info
// in ink-2 and debug in ink-3.
const LEVEL_CLASS = {
  ERROR: "text-below font-medium",
  WARN: "text-warning font-medium",
  INFO: "text-muted-foreground",
  DEBUG: "text-faint",
};

const STATUS = {
  connected: { label: "Live", dot: "bg-inflow" },
  connecting: { label: "Connecting…", dot: "border border-faint" },
  error: { label: "Disconnected", dot: "bg-below" },
};

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

  const state = STATUS[status] ?? STATUS.error;

  return (
    <Page>
      <Section
        title="Server log"
        description={
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden="true" className={cn("size-2 rounded-full", state.dot)} />
            {state.label}
          </span>
        }
        action={
          <>
            <NativeSelect
              size="sm"
              aria-label="Minimum level"
              value={minLevel}
              onChange={(e) => setMinLevel(e.target.value)}
              className="w-28"
            >
              {LEVELS.map((level) => (
                <NativeSelectOption key={level} value={level}>{level}</NativeSelectOption>
              ))}
            </NativeSelect>
            <Button variant="ghost" size="sm" onClick={() => setEntries([])}>
              Clear
            </Button>
          </>
        }
      >
        {streamError && <Notice variant="critical">{streamError.message || String(streamError)}</Notice>}

        <div
          ref={listRef}
          data-slot="log-stream"
          className="h-[60vh] overflow-auto rounded-lg bg-muted px-3 py-2 font-mono text-code leading-relaxed"
        >
          {entries.length === 0 ? (
            <p className="text-faint">Waiting for log entries…</p>
          ) : (
            <ul className="flex flex-col">
              {entries.map((entry, idx) => (
                <li
                  key={`${entry.time}-${idx}`}
                  data-level={entry.level}
                  className="grid grid-cols-[auto_5ch_minmax(0,1fr)] gap-x-3 py-0.5 max-sm:grid-cols-[5ch_minmax(0,1fr)]"
                >
                  <span className="whitespace-nowrap text-faint max-sm:col-span-2">{entry.time}</span>
                  <span className={LEVEL_CLASS[entry.level] ?? "text-muted-foreground"}>{entry.level}</span>
                  <span className="break-all">
                    {entry.message}
                    {formatAttrs(entry.attrs) && (
                      <span className="text-faint"> {formatAttrs(entry.attrs)}</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <p className="text-control text-faint">Keeps the latest 400 entries. The stream closes when you leave this page.</p>
      </Section>
    </Page>
  );
}
