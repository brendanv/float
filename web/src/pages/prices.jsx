import { useState } from "react";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

function today() {
  return new Date().toISOString().slice(0, 10);
}

export function PricesPage() {
  const prices = useRpc(() => ledgerClient.listPrices({}), []);

  const [date, setDate] = useState(today);
  const [commodity, setCommodity] = useState("");
  const [quantity, setQuantity] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState(null);

  async function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    setSubmitting(true);
    try {
      await ledgerClient.addPrice({ date, commodity: commodity.trim(), quantity: quantity.trim(), currency: currency.trim() });
      setCommodity("");
      setQuantity("");
      prices.refetch();
    } catch (err) {
      setFormError(err);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(pid) {
    try {
      await ledgerClient.deletePrice({ pid });
      prices.refetch();
    } catch (err) {
      setFormError(err);
    }
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
            <button type="submit" class="btn btn-primary btn-sm" disabled={submitting}>
              {submitting ? "Adding…" : "Add"}
            </button>
          </form>
          {formError && <ErrorBanner error={formError} />}
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Price History</h3>
          {prices.loading && <Loading />}
          {prices.error && <ErrorBanner error={prices.error} />}
          {prices.data && (
            prices.data.prices?.length > 0 ? (
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
                    {[...prices.data.prices].reverse().map((p) => (
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
