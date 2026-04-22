import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
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
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";

export function AccountsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
  });

  const [name, setName] = useState("");
  const [formError, setFormError] = useState(null);

  const addMutation = useMutation({
    mutationFn: (vars) => ledgerClient.declareAccount(vars),
    onSuccess: () => {
      setName("");
      setFormError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() });
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ aid }) => ledgerClient.deleteAccountDeclaration({ aid }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() }),
    onError: (err) => setFormError(err),
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ name: name.trim() });
  }

  const sorted = [...(data?.declarations ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name)
  );

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Account Declarations</h2>

      <Card>
        <CardHeader>
          <CardTitle>Declare Account</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
            <div className="w-full flex flex-col gap-1.5 sm:w-72">
              <Label htmlFor="acct-name">Account Name</Label>
              <Input
                id="acct-name"
                type="text"
                placeholder="assets:checking"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={addMutation.isPending}>
              {addMutation.isPending && (
                <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
              )}
              {addMutation.isPending ? "Declaring…" : "Declare"}
            </Button>
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Declared Accounts</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {data && (
            sorted.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Account</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sorted.map((d) => (
                    <TableRow key={d.aid || d.name}>
                      <TableCell className="font-mono">{d.name}</TableCell>
                      <TableCell>
                        {d.aid && (
                          <Button
                            variant="ghost"
                            size="xs"
                            className="text-destructive"
                            onClick={() => deleteMutation.mutate({ aid: d.aid })}
                          >
                            Delete
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <p className="text-sm text-muted-foreground">No account declarations yet.</p>
            )
          )}
        </CardContent>
      </Card>
    </div>
  );
}
