import { useState, useEffect, useRef } from "react";

export function useRpc(rpcFn, deps = []) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const counterRef = useRef(0);

  function fetch() {
    const id = ++counterRef.current;
    setLoading(true);
    setError(null);
    rpcFn()
      .then((result) => {
        if (id === counterRef.current) {
          setData(result);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (id === counterRef.current) {
          setError(err);
          setLoading(false);
        }
      });
  }

  useEffect(fetch, deps);

  return { data, loading, error, refetch: fetch };
}
