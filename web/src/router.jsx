import { useState, useEffect } from "react";

function parseHash() {
  const hash = window.location.hash.slice(1) || "/";
  const qIdx = hash.indexOf("?");
  const path = qIdx >= 0 ? hash.slice(0, qIdx) : hash;
  const params = {};
  if (qIdx >= 0) {
    new URLSearchParams(hash.slice(qIdx + 1)).forEach((v, k) => {
      params[k] = v;
    });
  }
  return { path, params };
}

export function navigate(path) {
  window.location.hash = "#" + path;
}

export function useRoute() {
  const [route, setRoute] = useState(parseHash);

  useEffect(() => {
    const onHashChange = () => setRoute(parseHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  return route;
}
