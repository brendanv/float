import { navigate } from "../router.jsx";

function NavLink({ href, label, current }) {
  const active = current === href;
  return (
    <li>
      <a
        href={"#" + href}
        class={active ? "contrast" : "secondary"}
        onClick={(e) => {
          e.preventDefault();
          navigate(href);
        }}
      >
        {label}
      </a>
    </li>
  );
}

export function AppShell({ children, currentPath }) {
  return (
    <div>
      <nav class="container">
        <ul>
          <li>
            <a href="#/" onClick={(e) => { e.preventDefault(); navigate("/"); }} style="display:flex;align-items:center;gap:0.4em">
              <img src="/icon.png" alt="" style="height:1.75em;width:1.75em;border-radius:0.3em" />
              <strong>float</strong>
            </a>
          </li>
        </ul>
        <ul>
          <NavLink href="/" label="Home" current={currentPath} />
          <NavLink href="/transactions" label="Transactions" current={currentPath} />
          <NavLink href="/add" label="Add" current={currentPath} />
        </ul>
      </nav>
      <main class="container">
        {children}
      </main>
    </div>
  );
}
