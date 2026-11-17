import { useState, useRef } from "react";
import { useMutation } from "@tanstack/react-query";
import { Play, ChevronRight } from "lucide-react";
import { ledgerClient } from "../client.js";
import { Page, Section, Eyebrow } from "../components/page.jsx";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Notice } from "@/components/ui/alert";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

const HLEDGER_EXAMPLES = [
  { label: "Account balances", args: "bal" },
  { label: "Balance sheet (depth 2)", args: "bs --depth 2" },
  { label: "Income statement", args: "is --monthly" },
  { label: "Recent transactions", args: "print date:lastmonth.." },
  { label: "All accounts", args: "accounts --tree" },
  { label: "Tags in use", args: "tags" },
];

const NL_EXAMPLES = [
  "How much did I spend last month?",
  "What were my top expense categories this year?",
  "How much did I spend on dining out last quarter?",
  "What's my current net worth?",
  "Show me my income vs expenses for this year",
];

// Kbd is a keyboard shortcut chip.
function Kbd({ children }) {
  return (
    <kbd className="rounded-sm border border-input px-1 font-mono text-[10.5px] leading-4 text-muted-foreground">
      {children}
    </kbd>
  );
}

// Output is a block of raw hledger output: mono on a sunken surface, or on
// the below tint for stderr.
function Output({ children, tone = "ink" }) {
  return (
    <pre
      className={cn(
        "overflow-x-auto rounded-lg px-3 py-2.5 font-mono text-code leading-relaxed whitespace-pre",
        tone === "below" ? "bg-below-soft text-below" : "bg-muted",
      )}
    >
      {children}
    </pre>
  );
}

// RunState is the result's status: an accent or below dot and a label.
function RunState({ ok }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span aria-hidden="true" className={cn("size-2 rounded-full", ok ? "bg-primary" : "bg-below")} />
      {ok ? "Success" : "Error"}
    </span>
  );
}

// Examples is a row of quiet example chips that fill the input.
function Examples({ items, onPick, mono = false }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Eyebrow className="mr-1">Examples</Eyebrow>
      {items.map((ex) => (
        <Button
          key={ex.value}
          variant="outline"
          size="xs"
          className={cn("font-normal text-muted-foreground hover:text-foreground", mono && "font-mono text-[12px]")}
          title={ex.title}
          onClick={() => onPick(ex.value)}
        >
          {ex.value}
        </Button>
      ))}
    </div>
  );
}

function HledgerTab() {
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
      <div className="flex max-w-3xl flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="hledger-args" className="text-control">Arguments</Label>
          <div className="flex gap-2">
            <Textarea
              id="hledger-args"
              ref={textareaRef}
              value={args}
              onChange={(e) => setArgs(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="bal --depth 2 assets"
              className="min-h-0 resize-none font-mono text-code"
              rows={2}
            />
            <Button
              onClick={run}
              disabled={!args.trim()}
              isLoading={isPending}
              loadingText="Running…"
              className="shrink-0 self-start"
            >
              <Play data-icon="inline-start" />
              Run
            </Button>
          </div>
          <p className="text-control text-faint">
            <Kbd>Ctrl+Enter</Kbd> runs. Leave out <code className="font-mono text-code text-muted-foreground">hledger</code> and{" "}
            <code className="font-mono text-code text-muted-foreground">-f &lt;journal&gt;</code>.
          </p>
        </div>
        <Examples
          items={HLEDGER_EXAMPLES.map((ex) => ({ value: ex.args, title: ex.label }))}
          onPick={applyExample}
          mono
        />
      </div>

      {error && <Notice variant="critical">{error.message || String(error)}</Notice>}

      {hasResult && (
        <Section title="Output" description={<RunState ok={success} />}>
          <p className="font-mono text-code break-all text-muted-foreground">$ {cmdLine}</p>
          {stdout && (
            <div className="flex flex-col gap-1.5">
              {stderr && <Eyebrow>stdout</Eyebrow>}
              <Output>{stdout}</Output>
            </div>
          )}
          {stderr && (
            <div className="flex flex-col gap-1.5">
              {stdout && <Eyebrow>stderr</Eyebrow>}
              <Output tone="below">{stderr}</Output>
            </div>
          )}
          {!stdout && !stderr && (
            <p className="text-control text-muted-foreground">(no output)</p>
          )}
        </Section>
      )}
    </div>
  );
}

function NaturalLanguageTab() {
  const [question, setQuestion] = useState("");
  const [detailsOpen, setDetailsOpen] = useState(false);
  const textareaRef = useRef(null);

  const { mutate, data, isPending, error, reset } = useMutation({
    mutationFn: (q) => ledgerClient.askQuestion({ question: q }),
    onSuccess: () => setDetailsOpen(false),
  });

  function ask() {
    const trimmed = question.trim();
    if (!trimmed) return;
    mutate(trimmed);
  }

  function handleKeyDown(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      ask();
    }
  }

  function applyExample(q) {
    setQuestion(q);
    reset();
    textareaRef.current?.focus();
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex max-w-3xl flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="nl-question" className="text-control">Question</Label>
          <div className="flex gap-2">
            <Textarea
              id="nl-question"
              ref={textareaRef}
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="How much did I spend on groceries last month?"
              className="min-h-0 resize-none"
              rows={2}
            />
            <Button
              onClick={ask}
              disabled={!question.trim()}
              isLoading={isPending}
              loadingText="Running…"
              className="shrink-0 self-start"
            >
              <Play data-icon="inline-start" />
              Run
            </Button>
          </div>
          <p className="text-control text-faint">
            <Kbd>Ctrl+Enter</Kbd> runs. The AI turns your question into an hledger query and answers from its output.
          </p>
        </div>
        <Examples items={NL_EXAMPLES.map((q) => ({ value: q }))} onPick={applyExample} />
      </div>

      {error && <Notice variant="critical">{error.message || String(error)}</Notice>}

      {data && (
        <Section title="Answer">
          <p className="max-w-3xl text-body leading-relaxed">{data.answer}</p>
          {!data.querySuccess && (
            <Notice variant="warning">The hledger query did not run successfully.</Notice>
          )}
          <div className="flex flex-col gap-3">
            <button
              type="button"
              aria-expanded={detailsOpen}
              className="touch-target inline-flex w-fit items-center gap-1.5 rounded-sm text-control text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring"
              onClick={() => setDetailsOpen((o) => !o)}
            >
              <ChevronRight className={cn("size-3.5 text-faint transition-transform duration-150", detailsOpen && "rotate-90")} />
              Query details
            </button>
            {detailsOpen && (
              <div className="flex flex-col gap-3 pl-5">
                <div className="flex flex-col gap-1.5">
                  <Eyebrow>Command</Eyebrow>
                  <Output>hledger {data.hledgerArgs}</Output>
                </div>
                {data.rawOutput && (
                  <div className="flex flex-col gap-1.5">
                    <Eyebrow>Raw output</Eyebrow>
                    <Output>{data.rawOutput}</Output>
                  </div>
                )}
              </div>
            )}
          </div>
        </Section>
      )}
    </div>
  );
}

export function HledgerQueryPage() {
  return (
    <Page>
      <Tabs defaultValue="hledger" className="gap-6">
        <TabsList>
          <TabsTrigger value="hledger">hledger</TabsTrigger>
          <TabsTrigger value="natural-language">Natural language</TabsTrigger>
        </TabsList>
        <TabsContent value="hledger">
          <HledgerTab />
        </TabsContent>
        <TabsContent value="natural-language">
          <NaturalLanguageTab />
        </TabsContent>
      </Tabs>
    </Page>
  );
}
