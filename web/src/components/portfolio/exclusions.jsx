import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { XIcon } from "@phosphor-icons/react";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function usePortfolioExclusions() {
  const queryClient = useQueryClient();

  const { data: settings } = useQuery({
    queryKey: queryKeys.portfolioSettings(),
    queryFn: () => ledgerClient.getPortfolioSettings({}),
  });

  const updateMutation = useMutation({
    mutationFn: (data) => ledgerClient.updatePortfolioSettings(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolioSettings() });
      queryClient.invalidateQueries({ queryKey: ["portfolioHoldings"] });
      queryClient.invalidateQueries({ queryKey: ["portfolioTimeseries"] });
    },
  });

  const excludedSymbols = settings?.excludedSymbols ?? [];
  const excludedAccountPrefixes = settings?.excludedAccountPrefixes ?? [];
  const totalExclusions = excludedSymbols.length + excludedAccountPrefixes.length;

  function update(symbols, prefixes) {
    updateMutation.mutate({ excludedSymbols: symbols, excludedAccountPrefixes: prefixes });
  }

  return { excludedSymbols, excludedAccountPrefixes, totalExclusions, update, isPending: updateMutation.isPending };
}

export function ExclusionsDialog({ open, onOpenChange, accountPrefix, onAccountPrefixChange }) {
  const { excludedSymbols, excludedAccountPrefixes, update, isPending } = usePortfolioExclusions();
  const [newSymbol, setNewSymbol] = useState("");
  const [newPrefix, setNewPrefix] = useState("");

  function addSymbol() {
    const s = newSymbol.trim().toUpperCase();
    if (!s || excludedSymbols.includes(s)) return;
    update([...excludedSymbols, s], excludedAccountPrefixes);
    setNewSymbol("");
  }

  function removeSymbol(sym) {
    update(excludedSymbols.filter((s) => s !== sym), excludedAccountPrefixes);
  }

  function addPrefix() {
    const s = newPrefix.trim();
    if (!s || excludedAccountPrefixes.includes(s)) return;
    update(excludedSymbols, [...excludedAccountPrefixes, s]);
    setNewPrefix("");
  }

  function removePrefix(prefix) {
    update(excludedSymbols, excludedAccountPrefixes.filter((p) => p !== prefix));
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>Portfolio Filters</DialogTitle>
          <DialogDescription>
            Scope and exclusions applied to holdings, charts, and totals.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-5">
          <div>
            <Label htmlFor="dialog-account-prefix" className="mb-2 block">Account Scope</Label>
            <Input
              id="dialog-account-prefix"
              className="h-8 w-full font-mono text-sm"
              placeholder="assets (default)"
              value={accountPrefix}
              onChange={(e) => onAccountPrefixChange(e.target.value)}
            />
            <p className="mt-1.5 text-xs text-muted-foreground">
              Show only holdings under this account prefix, e.g. <code className="font-mono">assets:investments</code>.
            </p>
          </div>

          <div className="border-t border-border pt-4">
            <Label className="mb-2 block">Excluded Commodities</Label>
            <div className="flex gap-2 mb-2">
              <Input
                className="h-8 flex-1 font-mono text-sm"
                placeholder="e.g. HOUSE, BTC"
                value={newSymbol}
                onChange={(e) => setNewSymbol(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); addSymbol(); } }}
                disabled={isPending}
              />
              <Button size="sm" onClick={addSymbol} disabled={isPending || !newSymbol.trim()}>
                Add
              </Button>
            </div>
            {excludedSymbols.length === 0 ? (
              <p className="text-xs text-muted-foreground">No excluded commodities.</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {excludedSymbols.map((sym) => (
                  <Badge key={sym} variant="secondary" className="gap-1 font-mono pr-1">
                    {sym}
                    <button
                      type="button"
                      className="ml-0.5 rounded opacity-60 hover:opacity-100 hover:text-destructive"
                      onClick={() => removeSymbol(sym)}
                      disabled={isPending}
                    >
                      <XIcon size={10} />
                    </button>
                  </Badge>
                ))}
              </div>
            )}
          </div>

          <div>
            <Label className="mb-2 block">Excluded Account Prefixes</Label>
            <div className="flex gap-2 mb-2">
              <Input
                className="h-8 flex-1 font-mono text-sm"
                placeholder="e.g. assets:property"
                value={newPrefix}
                onChange={(e) => setNewPrefix(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); addPrefix(); } }}
                disabled={isPending}
              />
              <Button size="sm" onClick={addPrefix} disabled={isPending || !newPrefix.trim()}>
                Add
              </Button>
            </div>
            {excludedAccountPrefixes.length === 0 ? (
              <p className="text-xs text-muted-foreground">No excluded account prefixes.</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {excludedAccountPrefixes.map((prefix) => (
                  <Badge key={prefix} variant="secondary" className="gap-1 font-mono pr-1">
                    {prefix}
                    <button
                      type="button"
                      className="ml-0.5 rounded opacity-60 hover:opacity-100 hover:text-destructive"
                      onClick={() => removePrefix(prefix)}
                      disabled={isPending}
                    >
                      <XIcon size={10} />
                    </button>
                  </Badge>
                ))}
              </div>
            )}
          </div>
        </div>

        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  );
}
