import { useState, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Loader2, ExternalLink } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

export function PayeesPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const {
    data: payeesData,
    isLoading: payeesLoading,
    error: payeesError,
  } = useQuery({
    queryKey: queryKeys.payees(),
    queryFn: () => ledgerClient.listPayees({}),
  });

  const {
    data: txData,
    isLoading: txLoading,
    error: txError,
  } = useQuery({
    queryKey: queryKeys.noPayeeTransactions(),
    queryFn: () => ledgerClient.listTransactions({ query: ["not:payee:.+"] }),
  });

  // Group unassigned transactions by description, sorted by frequency
  const descGroups = useMemo(() => {
    const map = new Map();
    for (const tx of txData?.transactions ?? []) {
      if (!map.has(tx.description)) map.set(tx.description, []);
      map.get(tx.description).push(tx.fid);
    }
    return [...map.entries()].sort((a, b) => b[1].length - a[1].length);
  }, [txData]);

  const [payeeFilter, setPayeeFilter] = useState("");

  const filteredPayees = useMemo(() => {
    const q = payeeFilter.trim().toLowerCase();
    if (!q) return payeesData?.payees ?? [];
    return (payeesData?.payees ?? []).filter((p) =>
      p.toLowerCase().includes(q)
    );
  }, [payeesData, payeeFilter]);

  // Which description row is currently being assigned a payee
  const [activeDesc, setActiveDesc] = useState(null);
  const [newPayee, setNewPayee] = useState("");
  const [settingPayee, setSettingPayee] = useState(false);
  const [setPayeeError, setSetPayeeError] = useState(null);

  function openSetPayee(desc) {
    setActiveDesc(desc);
    setNewPayee("");
    setSetPayeeError(null);
  }

  function cancelSetPayee() {
    setActiveDesc(null);
    setNewPayee("");
    setSetPayeeError(null);
  }

  async function confirmSetPayee(fids) {
    if (!newPayee.trim()) return;
    setSettingPayee(true);
    setSetPayeeError(null);
    try {
      await ledgerClient.bulkEditTransactions({
        fids,
        operations: [
          { operation: { case: "setPayee", value: { payee: newPayee.trim() } } },
        ],
      });
      setActiveDesc(null);
      setNewPayee("");
      queryClient.invalidateQueries({ queryKey: ["transactions"] });
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: queryKeys.noPayeeTransactions() });
      queryClient.invalidateQueries({ queryKey: queryKeys.payees() });
    } catch (err) {
      setSetPayeeError(err.message || String(err));
    } finally {
      setSettingPayee(false);
    }
  }

  const isLoading = payeesLoading || txLoading;
  const error = payeesError || txError;

  if (isLoading) return <Loading />;
  if (error) return <ErrorBanner error={error} />;

  return (
    <div className="flex flex-col gap-8">
      {/* Section 1: Explicit payees */}
      <Card>
        <CardHeader>
          <CardTitle>Payees</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <Input
            placeholder="Filter payees…"
            value={payeeFilter}
            onChange={(e) => setPayeeFilter(e.target.value)}
            className="max-w-sm"
          />
          {filteredPayees.length === 0 ? (
            <p className="text-sm text-muted-foreground">No payees found.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Payee</TableHead>
                  <TableHead className="w-36"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredPayees.map((payee) => (
                  <TableRow key={payee}>
                    <TableCell className="font-medium">{payee}</TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="gap-1.5"
                        onClick={() =>
                          navigate({ to: "/transactions", search: { payee } })
                        }
                      >
                        <ExternalLink className="size-3.5" />
                        View transactions
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Section 2: Common descriptions without payees */}
      <Card>
        <CardHeader>
          <CardTitle>Common descriptions without a payee</CardTitle>
        </CardHeader>
        <CardContent>
          {descGroups.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              All transactions have a payee assigned.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Description</TableHead>
                  <TableHead className="w-24">Count</TableHead>
                  <TableHead className="w-72"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {descGroups.map(([desc, fids]) => (
                  <TableRow key={desc}>
                    <TableCell className="font-mono text-sm">{desc}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{fids.length}</Badge>
                    </TableCell>
                    <TableCell>
                      {activeDesc === desc ? (
                        <div className="flex flex-col gap-2">
                          <div className="flex items-center gap-2">
                            <Input
                              autoFocus
                              placeholder="Payee name"
                              value={newPayee}
                              onChange={(e) => setNewPayee(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") confirmSetPayee(fids);
                                if (e.key === "Escape") cancelSetPayee();
                              }}
                              className="h-7 text-sm"
                            />
                            <Button
                              size="xs"
                              disabled={settingPayee || !newPayee.trim()}
                              onClick={() => confirmSetPayee(fids)}
                            >
                              {settingPayee ? (
                                <Loader2 className="size-3 animate-spin" />
                              ) : (
                                "Set"
                              )}
                            </Button>
                            <Button
                              variant="ghost"
                              size="xs"
                              disabled={settingPayee}
                              onClick={cancelSetPayee}
                            >
                              Cancel
                            </Button>
                          </div>
                          {setPayeeError && (
                            <p className="text-xs text-destructive">
                              {setPayeeError}
                            </p>
                          )}
                        </div>
                      ) : (
                        <div className="flex items-center gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openSetPayee(desc)}
                          >
                            Set payee
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="gap-1.5"
                            onClick={() =>
                              navigate({
                                to: "/transactions",
                                search: { search: desc },
                              })
                            }
                          >
                            <ExternalLink className="size-3.5" />
                            View
                          </Button>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
