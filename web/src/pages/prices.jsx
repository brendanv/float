import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

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
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Commodity Prices</h2>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Add Price</h3>
          <form onSubmit={handleSubmit} class="flex flex-wrap gap-3 items-end">
            <label class="form-control w-full sm:w-36">
              <div class="label"><span class="label-text">Date</span></div>
              <input
                type="date"
                class="input input-bordered input-sm"
                value={date}
                onInput={(e) => setDate(e.target.value)}
                required
              />
            </label>
            <label class="form-control w-full sm:w-32">
              <div class="label"><span class="label-text">Commodity</span></div>
              <input
                type="text"
                class="input input-bordered input-sm"
                placeholder="AAPL"
                value={commodity}
                onInput={(e) => setCommodity(e.target.value)}
                required
              />
            </label>
            <label class="form-control w-full sm:w-32">
              <div class="label"><span class="label-text">Price</span></div>
              <input
                type="text"
                class="input input-bordered input-sm"
                placeholder="178.50"
                value={quantity}
                onInput={(e) => setQuantity(e.target.value)}
                required
              />
            </label>
            <label class="form-control w-full sm:w-24">
              <div class="label"><span class="label-text">Currency</span></div>
              <input
                type="text"
                class="input input-bordered input-sm"
                value={currency}
                onInput={(e) => setCurrency(e.target.value)}
                required
              />
            </label>
            <button type="submit" class="btn btn-primary btn-sm" disabled={addMutation.isPending}>
              {addMutation.isPending ? "Adding…" : "Add"}
            </button>
          </form>
          {formError && <ErrorBanner error={formError} />}
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Price History</h3>
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {pricesData && (
            pricesData.prices?.length > 0 ? (
              <div class="overflow-x-auto">
                <table class="table table-sm">
                  <thead>
                    <tr>
                      <th>Date</th>
                      <th>Commodity</th>
                      <th>Price</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {[...pricesData.prices].reverse().map((p) => (
                      <tr key={p.pid}>
                        <td class="font-mono">{p.date}</td>
                        <td class="font-mono font-semibold">{p.commodity}</td>
                        <td class="font-mono">{p.price?.quantity} {p.price?.commodity}</td>
                        <td>
                          {p.pid && (
                            <button
                              class="btn btn-ghost btn-xs text-error"
                              onClick={() => handleDelete(p.pid)}
                            >
                              Delete
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p class="text-base-content/50 text-sm">No prices recorded yet.</p>
            )
          )}
        </div>
      </div>
    </div>
  );
}
