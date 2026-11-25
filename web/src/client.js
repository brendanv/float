import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { LedgerService } from "./gen/float/v1/ledger_pb.js";
import { SNAPSHOT_TRAILER, reportUndoable } from "./lib/undo.js";

// Redirect to the login page whenever the server rejects a call as
// unauthenticated. The session cookie set by /api/login rides along on
// same-origin requests, so no header handling is needed here.
const redirectToLogin = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      !window.location.hash.startsWith("#/login")
    ) {
      window.location.hash = "#/login";
    }
    throw err;
  }
};

// Report mutations that floatd says can be undone (the float-snapshot
// trailer). A streaming response's trailer only arrives after its last
// message, so the stream is wrapped and checked once it has been drained.
const reportUndoableChanges = (next) => async (req) => {
  const res = await next(req);
  const method = req.method.name;
  if (!res.stream) {
    const hash = res.trailer.get(SNAPSHOT_TRAILER);
    if (hash) reportUndoable({ method, hash, request: req.message });
    return res;
  }
  const messages = res.message;
  return {
    ...res,
    message: (async function* () {
      yield* messages;
      const hash = res.trailer.get(SNAPSHOT_TRAILER);
      if (hash) reportUndoable({ method, hash });
    })(),
  };
};

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  interceptors: [redirectToLogin, reportUndoableChanges],
});

export const ledgerClient = createClient(LedgerService, transport);
