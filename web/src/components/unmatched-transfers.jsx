import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { Account } from "./account.jsx";
import { Amount } from "./amount.jsx";
import { ErrorBanner } from "./error-banner.jsx";
import { Section } from "./page.jsx";
import { formatDate } from "../format.js";
import { Notice } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useMissingTransferCount, useUnmatchedTransfers } from "@/hooks/use-unmatched-transfers.js";
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

function TransferRow({ transfer, account }) {
  const tx = transfer.transaction;
  const source = sourcePosting(tx, account);
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

// UnmatchedTransfersSection lists transfer sides through the clearing account
// with no matching other side. It renders nothing when every transfer has
// both sides.
export function UnmatchedTransfersSection() {
  const { data, error } = useUnmatchedTransfers();
  if (error) return <ErrorBanner error={error} />;
  const transfers = data?.transfers || [];
  if (transfers.length === 0) return null;
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
    >
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
          {transfers.map((t, i) => (
            <TransferRow key={t.transaction.fid || i} transfer={t} account={account} />
          ))}
        </TableBody>
      </Table>
    </Section>
  );
}

// MissingTransfersNotice points at Settings › Assertions when a transfer has
// been missing its other side for longer than the match window.
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
