export function ErrorBanner({ error }) {
  if (!error) return null;
  return (
    <article style={{ background: "var(--pico-del-color)", color: "white", padding: "0.75rem 1rem", marginBottom: "1rem" }}>
      {error.message || String(error)}
    </article>
  );
}
