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
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";

function today() {
  return new Date().toISOString().slice(0, 10);
}

export function PricesPage() {
  const queryClient = useQueryClient();

  const { data: pricesData, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.prices(),
    queryFn: () => ledgerClient.listPrices({}),
  });

  const [date, setDate] = useState(today);
  const [commodity, setCommodity] = useState("");
  const [quantity, setQuantity] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [formError, setFormError] = useState(null);

  const addMutation = useMutation({
    mutationFn: (vars) => ledgerClient.addPrice(vars),
    onSuccess: () => {
      setCommodity("");
      setQuantity("");
      setFormError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.prices() });
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ pid }) => ledgerClient.deletePrice({ pid }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.prices() }),
    onError: (err) => setFormError(err),
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ date, commodity: commodity.trim(), quantity: quantity.trim(), currency: currency.trim() });
  }

  function handleDelete(pid) {
    deleteMutation.mutate({ pid });
  }

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold">Commodity Prices</h2>

      <Card>
        <CardHeader>
          <CardTitle>Add Price</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
            <div className="w-full space-y-1.5 sm:w-36">
              <Label htmlFor="price-date">Date</Label>
              <Input
                id="price-date"
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                required
              />
            </div>
            <div className="w-full space-y-1.5 sm:w-32">
              <Label htmlFor="price-commodity">Commodity</Label>
              <Input
                id="price-commodity"
                type="text"
                placeholder="AAPL"
                value={commodity}
                onChange={(e) => setCommodity(e.target.value)}
                required
              />
            </div>
            <div className="w-full space-y-1.5 sm:w-32">
              <Label htmlFor="price-quantity">Price</Label>
              <Input
                id="price-quantity"
                type="text"
                placeholder="178.50"
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
                required
              />
            </div>
            <div className="w-full space-y-1.5 sm:w-24">
              <Label htmlFor="price-currency">Currency</Label>
              <Input
                id="price-currency"
                type="text"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={addMutation.isPending}>
              {addMutation.isPending ? "Adding…" : "Add"}
            </Button>
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Price History</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {pricesData && (
            pricesData.prices?.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Date</TableHead>
                    <TableHead>Commodity</TableHead>
                    <TableHead>Price</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {[...pricesData.prices].reverse().map((p) => (
                    <TableRow key={p.pid}>
                      <TableCell className="font-mono">{p.date}</TableCell>
                      <TableCell className="font-mono font-semibold">{p.commodity}</TableCell>
                      <TableCell className="font-mono">{p.price?.quantity} {p.price?.commodity}</TableCell>
                      <TableCell>
                        {p.pid && (
                          <Button
                            variant="ghost"
                            size="xs"
                            className="text-destructive"
                            onClick={() => handleDelete(p.pid)}
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
              <p className="text-sm text-muted-foreground">No prices recorded yet.</p>
            )
          )}
        </CardContent>
      </Card>
    </div>
  );
}
