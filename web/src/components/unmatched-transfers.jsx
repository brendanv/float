import { Fragment, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { Account } from "./account.jsx";
import { Amount } from "./amount.jsx";
import { ErrorBanner } from "./error-banner.jsx";
import { Section } from "./page.jsx";
import { formatDate } from "../format.js";
import { Notice } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  RECENT_MONTHS,
  recentCutoff,
  useMissingTransferCount,
  useUnmatchedTransfers,
} from "@/hooks/use-unmatched-transfers.js";
import { cn } from "@/lib/utils";

function inAccount(name, account) {
  return name === account || name.startsWith(`${account}:`);
}

// sourcePosting is the side's own account: the first posting outside the
// clearing account (what floatd matches on).
function sourcePosting(tx, account) {
  return tx.postings.find((p) => !inAccount(p.account, account));
}

function TransferStatus({ status }) {
  const missing = status === "missing";
  const label = missing ? "Missing other side" : "Recent";
  return (
    <span title={label} className={cn("inline-flex items-center gap-2 whitespace-nowrap", !missing && "text-muted-foreground")}>
      <span className="inline-flex size-2 shrink-0 items-center justify-center">
        {missing && <span aria-hidden="true" className="size-2 rounded-full bg-warning" />}
      </span>
      <span className="max-sm:sr-only">{missing ? "Missing" : label}</span>
    </span>
  );
}

// routeLabel names a route by its segment under the clearing account
// ("Schwab-Amex"); direct postings to the clearing account itself have none.
function routeLabel(route, account) {
  return route === account ? "Direct" : route.slice(account.length + 1);
}

// orList joins account names as "a, b or c".
function orList(names) {
  if (names.length <= 1) return names[0] || "";
  return `${names.slice(0, -1).join(", ")} or ${names.at(-1)}`;
}

// RouteHeader heads one route's rows with its name.
function RouteHeader({ route, account }) {
  return (
    <tr data-slot="transfer-route" className="shadow-[inset_0_-1px_0_var(--input)]">
      <td colSpan={5} className="px-2 pt-5 pb-1.5 font-medium" title={route}>
        {routeLabel(route, account)}
      </td>
    </tr>
  );
}

function TransferRow({ transfer, account }) {
  const tx = transfer.transaction;
  const source = sourcePosting(tx, account);
  const expected = transfer.expectedAccounts || [];
  return (
    <TableRow data-slot="unmatched-transfer-row">
      <TableCell className="w-6 pr-0 sm:w-28 sm:pr-2">
        <TransferStatus status={transfer.status} />
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground">{formatDate(tx.date)}</TableCell>
      <TableCell className="w-full max-w-0">
        {tx.fid ? (
          <Link
            to="/transactions"
            search={{ search: `code:${tx.fid}` }}
            className="block truncate hover:underline"
          >
            {tx.description}
          </Link>
        ) : (
          <span className="block truncate">{tx.description}</span>
        )}
        {expected.length > 0 && (
          <span data-slot="expected-accounts" className="block text-[12px] whitespace-normal text-muted-foreground [overflow-wrap:anywhere]">
            Expected on {orList(expected)}
          </span>
        )}
      </TableCell>
      <TableCell className="hidden sm:table-cell">
        {source ? <Account name={source.account} variant="path" /> : <span className="text-faint">—</span>}
      </TableCell>
      <TableCell className="text-right">
        <Amount value={source?.amounts?.length ? source.amounts : transfer.amount} />
      </TableCell>
    </TableRow>
  );
}

// groupByRoute groups transfers by route, routes ordered by their newest side
// (transfers arrive newest first).
function groupByRoute(transfers) {
  const groups = new Map();
  for (const t of transfers) {
    const key = t.route || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(t);
  }
  return [...groups.entries()];
}

// UnmatchedTransfersSection lists transfer sides through the clearing account
// with no matching other side, grouped by route (the clearing account's direct
// children, e.g. equity:transfers:Schwab-Amex). Only the last RECENT_MONTHS
// are listed until "Show all". It renders nothing when every transfer has both
// sides.
export function UnmatchedTransfersSection() {
  const { data, error } = useUnmatchedTransfers();
  const [showAll, setShowAll] = useState(false);
  if (error) return <ErrorBanner error={error} />;
  const all = data?.transfers || [];
  if (all.length === 0) return null;
  const cutoff = recentCutoff();
  const recent = all.filter((t) => t.transaction.date >= cutoff);
  const older = all.length - recent.length;
  const transfers = showAll ? all : recent;
  const account = data.account;
  return (
    <Section
      title="Unmatched transfers"
      description={
        <>
          Transfers through <span className="font-mono text-code">{account}</span> with no other side within{" "}
          {data.windowDays} days
        </>
      }
      action={
        older > 0 && (
          <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={() => setShowAll((v) => !v)}>
            {showAll ? `Last ${RECENT_MONTHS} months` : `Show all (${older} older)`}
          </Button>
        )
      }
    >
      {transfers.length === 0 ? (
        <p className="text-control text-muted-foreground">
          No unmatched transfers in the last {RECENT_MONTHS} months.
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-6 pr-0 sm:w-28 sm:pr-2"><span className="max-sm:sr-only">Status</span></TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Description</TableHead>
              <TableHead className="hidden sm:table-cell">Account</TableHead>
              <TableHead className="text-right">Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groupByRoute(transfers).map(([route, rows]) => (
              <Fragment key={route}>
                <RouteHeader route={route || account} account={account} />
                {rows.map((t, i) => (
                  <TransferRow key={t.transaction.fid || i} transfer={t} account={account} />
                ))}
              </Fragment>
            ))}
          </TableBody>
        </Table>
      )}
    </Section>
  );
}

// MissingTransfersNotice points at Settings › Assertions when a transfer from
// the last RECENT_MONTHS has been missing its other side for longer than the
// match window.
export function MissingTransfersNotice() {
  const count = useMissingTransferCount();
  if (!count) return null;
  return (
    <Notice
      variant="warning"
      action={
        <Link
          to="/assertions"
          className="touch-target inline-flex items-center gap-1 font-medium text-primary hover:underline"
        >
          Check
          <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
      }
    >
      <span className="font-medium">
        {count} {count === 1 ? "transfer is" : "transfers are"}
      </span>{" "}
      missing the other side — a transaction may not have been imported.
    </Notice>
  );
}
