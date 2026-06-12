package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/slogctx"
)

// GetGeneralConfig returns general configuration values (e.g. timezone).
func (h *Handler) GetGeneralConfig(ctx context.Context, req *connect.Request[floatv1.GetGeneralConfigRequest]) (*connect.Response[floatv1.GetGeneralConfigResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	return connect.NewResponse(&floatv1.GetGeneralConfigResponse{
		Timezone: cfg.Timezone,
	}), nil
}

// SetTimezone updates the timezone in config.toml. An empty string resets to UTC.
func (h *Handler) SetTimezone(ctx context.Context, req *connect.Request[floatv1.SetTimezoneRequest]) (*connect.Response[floatv1.SetTimezoneResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	tz := req.Msg.Timezone
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid timezone %q: %w", tz, err))
		}
	}

	err := h.lock.Do(ctx, "set timezone", func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.Timezone = tz
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set timezone failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated timezone", "timezone", tz)
	return connect.NewResponse(&floatv1.SetTimezoneResponse{}), nil
}
