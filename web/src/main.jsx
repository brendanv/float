import { render } from "preact";
import { useRoute } from "./router.jsx";
import { AppShell } from "./components/app-shell.jsx";
import { HomePage } from "./pages/home.jsx";
import { TransactionsPage } from "./pages/transactions.jsx";
import { AddTransactionPage } from "./pages/add-transaction.jsx";
import { TrendsPage } from "./pages/trends.jsx";
import { PricesPage } from "./pages/prices.jsx";
import { SnapshotsPage } from "./pages/snapshots.jsx";
import { ImportPage } from "./pages/import.jsx";
import { RulesPage } from "./pages/rules.jsx";

function App() {
  const { path, params } = useRoute();

  let page;
  switch (path) {
    case "/":
      page = <HomePage />;
      break;
    case "/transactions":
      page = <TransactionsPage params={params} />;
      break;
    case "/add":
      page = <AddTransactionPage />;
      break;
    case "/trends":
      page = <TrendsPage />;
      break;
    case "/prices":
      page = <PricesPage />;
      break;
    case "/snapshots":
      page = <SnapshotsPage />;
      break;
    case "/import":
      page = <ImportPage />;
      break;
    case "/rules":
      page = <RulesPage />;
      break;
    default:
      page = <p>Page not found. <a href="#/">Go home</a></p>;
      break;
  }

  return <AppShell currentPath={path}>{page}</AppShell>;
}

render(<App />, document.getElementById("app"));
