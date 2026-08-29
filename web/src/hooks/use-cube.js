import { useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";

import { queryKeys } from "../query-keys.js";
import { fetchGeneration, loadCube, StaleGenerationError } from "../lib/cube.js";
import { getGeneration, setGeneration, subscribeGeneration } from "../lib/generation.js";

/**
 * The server's current txlock generation.
 *
 * Two sources feed it: a one-off fetch for the very first render, before any
 * RPC has run, and the X-Float-Generation header every subsequent Connect
 * response carries. The header is what keeps it live — no polling.
 */
export function useGeneration() {
  const observed = useSyncExternalStore(subscribeGeneration, getGeneration, getGeneration);

  const { data: fetched } = useQuery({
    queryKey: queryKeys.generation(),
    queryFn: async () => {
      const generation = await fetchGeneration();
      setGeneration(generation);
      return generation;
    },
    // Only needed until some RPC has reported one.
    enabled: observed === null,
    staleTime: Infinity,
  });

  return observed ?? fetched ?? null;
}

/**
 * Loads the dashboard cube for the current generation.
 *
 * The payload is immutable for a given generation, so it never goes stale
 * within one: staleTime is Infinity, and the browser's HTTP cache serves repeat
 * loads without touching the server. When a write bumps the generation the key
 * changes and a new payload is fetched.
 *
 * A 409 means the generation moved between reading it and asking for the cube.
 * That is recorded and the query re-runs under the new key rather than
 * surfacing an error.
 */
export function useCube() {
  const generation = useGeneration();

  return useQuery({
    queryKey: queryKeys.cube(generation),
    queryFn: async () => {
      try {
        return await loadCube(generation);
      } catch (err) {
        if (err instanceof StaleGenerationError) {
          setGeneration(err.generation);
        }
        throw err;
      }
    },
    enabled: generation !== null,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: (count, err) => err instanceof StaleGenerationError && count < 2,
  });
}
