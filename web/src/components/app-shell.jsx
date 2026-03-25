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
            <a href="#/" onClick={(e) => { e.preventDefault(); navigate("/"); }}>
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
