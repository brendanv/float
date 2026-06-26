package ledger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/metabase"
	"github.com/brendanv/float/internal/slogctx"
)

// exportDBPath is where floatd writes the SQLite snapshot, relative to the data
// dir. The Metabase container reads the same file through its own mount point,
// configured separately as MetabaseConfig.DBPath.
func (h *Handler) exportDBPath() string {
	return filepath.Join(h.dataDir, "exports", "float.db")
}

// GetMetabaseConfig returns the Custom Dashboards / Metabase settings. The API
// key is never returned; only whether one is configured.
func (h *Handler) GetMetabaseConfig(ctx context.Context, req *connect.Request[floatv1.GetMetabaseConfigRequest]) (*connect.Response[floatv1.GetMetabaseConfigResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	mb := cfg.Metabase
	apiKeySet := mb.APIKey != ""
	configured := mb.Enabled && mb.URL != "" && apiKeySet && mb.DBPath != ""
	return connect.NewResponse(&floatv1.GetMetabaseConfigResponse{
		Enabled:    mb.Enabled,
		Url:        mb.URL,
		ApiUrl:     mb.APIURL,
		DbPath:     mb.DBPath,
		DbName:     mb.DBName,
		ApiKeySet:  apiKeySet,
		Configured: configured,
	}), nil
}

// SetMetabaseConfig persists the Custom Dashboards / Metabase settings. The API
// key is preserved when the request leaves it empty, unless clear_api_key is set.
func (h *Handler) SetMetabaseConfig(ctx context.Context, req *connect.Request[floatv1.SetMetabaseConfigRequest]) (*connect.Response[floatv1.SetMetabaseConfigResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.DoWith(ctx, "set metabase config", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.Metabase.Enabled = req.Msg.Enabled
		newCfg.Metabase.URL = req.Msg.Url
		newCfg.Metabase.APIURL = req.Msg.ApiUrl
		newCfg.Metabase.DBPath = req.Msg.DbPath
		newCfg.Metabase.DBName = req.Msg.DbName
		switch {
		case req.Msg.ApiKey != "":
			newCfg.Metabase.APIKey = req.Msg.ApiKey
		case req.Msg.ClearApiKey:
			newCfg.Metabase.APIKey = ""
		default:
			newCfg.Metabase.APIKey = cur.Metabase.APIKey
		}
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set metabase config failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated metabase config", "enabled", req.Msg.Enabled, "url", req.Msg.Url)
	return connect.NewResponse(&floatv1.SetMetabaseConfigResponse{}), nil
}

// PrepareDashboards regenerates the SQLite export from the current ledger,
// ensures Metabase has a database connection pointed at it, triggers a schema
// re-sync, and returns the browser URL to open. This is the one-button handoff.
func (h *Handler) PrepareDashboards(ctx context.Context, req *connect.Request[floatv1.PrepareDashboardsRequest]) (*connect.Response[floatv1.PrepareDashboardsResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	mb := cfg.Metabase
	if !mb.Enabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("metabase integration is not enabled"))
	}
	if mb.URL == "" || mb.APIKey == "" || mb.DBPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("metabase URL, API key, and database path must be configured"))
	}

	// Regenerate the SQLite snapshot. This writes a derived artifact (not a
	// journal file), so it does not go through txlock.
	stats, err := metabase.Export(ctx, h.hl, h.exportDBPath())
	if err != nil {
		return nil, rpcErr(ctx, err, "metabase export failed")
	}

	client := metabase.NewClient(mb.APIBaseURL(), mb.APIKey)
	dbID, err := client.EnsureDatabase(ctx, mb.DatabaseName(), mb.DBPath)
	if err != nil {
		return nil, rpcErr(ctx, err, "metabase ensure database failed")
	}
	if err := client.SyncSchema(ctx, dbID); err != nil {
		return nil, rpcErr(ctx, err, "metabase sync schema failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "prepared metabase dashboards", "postings", stats.PostingCount, "db_id", dbID)
	return connect.NewResponse(&floatv1.PrepareDashboardsResponse{
		OpenUrl:      mb.URL,
		PostingCount: int64(stats.PostingCount),
		GeneratedAt:  stats.GeneratedAt.UTC().Format(time.RFC3339),
	}), nil
}
