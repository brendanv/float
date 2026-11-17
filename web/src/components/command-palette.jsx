import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { PlusIcon, UserIcon } from "lucide-react";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";
import { filterAndSortAccountNames } from "@/lib/accounts.js";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Account } from "./account.jsx";
import { PAGE_ENTRIES } from "./nav-items.js";

// CommandPalette ("Jump to…") lists pages, accounts and payees. Selecting an
// account or payee opens the transactions list filtered to it, as the
// account and payee links elsewhere do. Lists load only while it is open.
export function CommandPalette({ open, onOpenChange, onNewTransaction }) {
  const navigate = useNavigate();

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
    enabled: open,
  });
  const { data: payeesData } = useQuery({
    queryKey: queryKeys.payees(),
    queryFn: () => ledgerClient.listPayees({}),
    enabled: open,
  });

  const accounts = filterAndSortAccountNames(accountsData?.accounts);
  const payees = payeesData?.payees ?? [];

  function go(options) {
    onOpenChange(false);
    navigate(options);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="top-[12svh] translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-lg"
      >
        <DialogTitle className="sr-only">Jump to</DialogTitle>
        <DialogDescription className="sr-only">
          Search pages, accounts and payees.
        </DialogDescription>
        <Command loop className="rounded-none">
          <CommandInput placeholder="Jump to a page, account or payee…" />
          <CommandList className="max-h-[min(60svh,24rem)] p-1">
            <CommandEmpty>No matches.</CommandEmpty>
            <CommandGroup heading="Pages">
              {PAGE_ENTRIES.map((entry) => (
                <CommandItem
                  key={entry.to}
                  value={`page ${entry.label}`}
                  onSelect={() => go({ to: entry.to })}
                >
                  <entry.icon className="text-muted-foreground" />
                  {entry.label}
                </CommandItem>
              ))}
              <CommandItem
                value="action New transaction"
                onSelect={() => { onOpenChange(false); onNewTransaction?.(); }}
              >
                <PlusIcon className="text-muted-foreground" />
                New transaction
              </CommandItem>
            </CommandGroup>
            {accounts.length > 0 && (
              <CommandGroup heading="Accounts">
                {accounts.map((name) => (
                  <CommandItem
                    key={name}
                    value={`account ${name}`}
                    onSelect={() => go({ to: "/transactions", search: { account: name } })}
                  >
                    <Account name={name} variant="path" />
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {payees.length > 0 && (
              <CommandGroup heading="Payees">
                {payees.map((name) => (
                  <CommandItem
                    key={name}
                    value={`payee ${name}`}
                    onSelect={() => go({ to: "/transactions", search: { payee: name } })}
                  >
                    <UserIcon className="text-muted-foreground" />
                    <span className="truncate">{name}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}
