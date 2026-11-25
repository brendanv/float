package ledger

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/logstream"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/suggestions"
	"github.com/brendanv/float/internal/txlock"
)

// Handler implements LedgerService RPCs by delegating to the hledger wrapper.
type Handler struct {
	floatv1connect.UnimplementedLedgerServiceHandler
	hl             *hledger.Client
	lock           *txlock.TxLock
	dataDir        string
	configPath     string
	cache          *cache.Cache[any] // nil = bypass cache
	recentAccounts *recentAccounts
	snap           *gitsnap.Repo
	cfg            atomic.Pointer[config.Config]
	logBroadcaster *logstream.Broadcaster
	// suggestions mirrors account-suggestions.json (see handler_ai_accounts.go).
	// Only the background suggester writes it, publishing after the save.
	suggestions atomic.Pointer[suggestions.Store]
	// suggestWake wakes the background suggester after a write (cap 1).
	suggestWake chan struct{}
	// AIBaseURL overrides the OpenRouter API endpoint. Set in tests only.
	AIBaseURL string
	// afterImportAllPreFetch is called between the pre-fetch phase and lock acquisition
	// in ImportAllStripeTransactions. Used in tests to simulate a concurrent import (e.g.,
	// daily auto-import writing the same transactions). Nil in production.
	afterImportAllPreFetch func()
	// afterImportPreFetch is called between the pre-fetch phase and lock acquisition
	// in ImportStripeTransactions. Used in tests to simulate a concurrent import.
	// Nil in production.
	afterImportPreFetch func()
	// afterDailyImportPreFetch is called between the pre-lock dedup phase and lock
	// acquisition in runDailyStripeImport. Used in tests to simulate a concurrent
	// import landing in the race window. Nil in production.
	afterDailyImportPreFetch func()
	// beforeApplyRulesLock is called in ApplyRules just before lock acquisition.
	// Used in tests to simulate a concurrent write landing before the rules are
	// matched and applied under the lock. Nil in production.
	beforeApplyRulesLock func()
}

func NewHandler(hl *hledger.Client, lock *txlock.TxLock, dataDir string, configPath string, c *cache.Cache[any], snap *gitsnap.Repo, cfg *config.Config, broadcaster *logstream.Broadcaster) *Handler {
	h := &Handler{hl: hl, lock: lock, dataDir: dataDir, configPath: configPath, cache: c, recentAccounts: newRecentAccounts(recentAccountsCap), snap: snap, logBroadcaster: broadcaster, suggestWake: make(chan struct{}, 1)}
	h.cfg.Store(cfg)
	h.loadSuggestions()
	return h
}

// loadCfg returns the current config snapshot. Readers call this once per
// request and work from the returned pointer. Writers read it as their base
// inside a txlock callback and publish a new immutable snapshot only once the
// transaction has committed (see saveCfg).
func (h *Handler) loadCfg() *config.Config {
	return h.cfg.Load()
}

// saveCfg writes newCfg to config.toml and returns a func that publishes it as
// the in-memory config. Call it from a lock.DoWithPublish/DoConfigPublish
// callback (with h.configPath declared) and return the publish func, so h.cfg
// only changes once the whole transaction — including the post-callback
// hledger check of DoWithPublish — has succeeded and the file write is final.
func (h *Handler) saveCfg(newCfg *config.Config) (publish func(), err error) {
	if err := config.Save(h.configPath, newCfg); err != nil {
		return nil, err
	}
	return func() { h.cfg.Store(newCfg) }, nil
}

func (h *Handler) RunHledgerQuery(ctx context.Context, req *connect.Request[floatv1.RunHledgerQueryRequest]) (*connect.Response[floatv1.RunHledgerQueryResponse], error) {
	logger := slogctx.FromContext(ctx)
	if strings.TrimSpace(req.Msg.Args) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("args is required"))
	}
	stdout, stderr, cmdLine, err := h.hl.RunQuery(ctx, req.Msg.Args)
	if errors.Is(err, hledger.ErrUnsafeQuery) {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	success := err == nil
	if err != nil {
		logger.InfoContext(ctx, "hledger query returned non-zero", "args", req.Msg.Args, "error", err)
	}
	return connect.NewResponse(&floatv1.RunHledgerQueryResponse{
		Stdout:      string(stdout),
		Stderr:      string(stderr),
		Success:     success,
		CommandLine: cmdLine,
	}), nil
}
