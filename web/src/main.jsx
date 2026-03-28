import { render } from "preact";
import { useRoute } from "./router.jsx";
import { AppShell } from "./components/app-shell.jsx";
import { HomePage } from "./pages/home.jsx";
import { TransactionsPage } from "./pages/transactions.jsx";
import { AddTransactionPage } from "./pages/add-transaction.jsx";
import { TrendsPage } from "./pages/trends.jsx";

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
    default:
      page = <p>Page not found. <a href="#/">Go home</a></p>;
      break;
  }

  return <AppShell currentPath={path}>{page}</AppShell>;
}

render(<App />, document.getElementById("app"));
