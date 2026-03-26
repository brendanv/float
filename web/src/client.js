import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { LedgerService } from "./gen/float/v1/ledger_pb.js";

const transport = createConnectTransport({
  baseUrl: window.location.origin,
});

export const ledgerClient = createClient(LedgerService, transport);
