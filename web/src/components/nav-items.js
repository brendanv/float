import {
  House,
  List,
  BarChart2,
  Briefcase,
  BookOpen,
  Users,
  ListFilter,
  TableProperties,
  Link2,
  Upload,
  Settings,
} from "lucide-react";

// Section tabs for nav items that group several routes. Every route stays
// reachable: the item links to its first tab, and SectionTabs (rendered by
// the shell at the top of the page) links to the rest.
export const SECTION_TABS = {
  reports: [
    { to: "/trends", label: "Net worth" },
    { to: "/monthly", label: "Income & spending" },
  ],
  investments: [
    { to: "/portfolio", label: "Holdings" },
    { to: "/lots", label: "Lots" },
    { to: "/prices", label: "Prices" },
  ],
  import: [
    { to: "/import", label: "Import" },
    { to: "/imports", label: "History" },
  ],
  settings: [
    { to: "/settings", label: "General" },
    { to: "/assertions", label: "Assertions" },
    { to: "/snapshots", label: "Snapshots" },
    { to: "/hledger-query", label: "Query" },
    { to: "/logs", label: "Logs" },
  ],
};

// Sidebar nav. `routes` lists every path the item counts as active for.
export const NAV_GROUPS = [
  {
    label: null,
    items: [
      { id: "overview", href: "/", label: "Overview", icon: House, routes: ["/"] },
      { id: "transactions", href: "/transactions", label: "Transactions", icon: List, routes: ["/transactions", "/add"] },
      { id: "reports", href: "/trends", label: "Reports", icon: BarChart2, tabs: SECTION_TABS.reports },
      { id: "investments", href: "/portfolio", label: "Investments", icon: Briefcase, tabs: SECTION_TABS.investments },
    ],
  },
  {
    label: "Organize",
    items: [
      { id: "accounts", href: "/accounts", label: "Accounts", icon: BookOpen },
      { id: "payees", href: "/payees", label: "Payees", icon: Users },
      { id: "rules", href: "/rules", label: "Rules", icon: ListFilter },
      { id: "templates", href: "/templates", label: "Templates", icon: TableProperties },
    ],
  },
  {
    label: "Bring in",
    items: [
      { id: "connections", href: "/connections", label: "Connections", icon: Link2 },
      { id: "import", href: "/import", label: "Import", icon: Upload, tabs: SECTION_TABS.import },
    ],
  },
];

export const SETTINGS_ITEM = {
  id: "settings", href: "/settings", label: "Settings", icon: Settings, tabs: SECTION_TABS.settings,
};

const ALL_ITEMS = [...NAV_GROUPS.flatMap((g) => g.items), SETTINGS_ITEM];

function itemRoutes(item) {
  return item.routes ?? item.tabs?.map((t) => t.to) ?? [item.href];
}

// navItemFor returns the nav item whose group contains `path`, if any.
export function navItemFor(path) {
  return ALL_ITEMS.find((item) => itemRoutes(item).includes(path));
}

// Every page, for the command palette: plain items plus each section tab.
export const PAGE_ENTRIES = ALL_ITEMS.flatMap((item) => {
  if (!item.tabs) return [{ to: item.href, label: item.label, icon: item.icon }];
  return item.tabs.map((tab) => ({
    to: tab.to,
    label: tab.label === item.label ? item.label : `${item.label} › ${tab.label}`,
    icon: item.icon,
  }));
});
