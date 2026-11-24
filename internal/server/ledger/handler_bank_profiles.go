package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/slogctx"
)

// ---- Import handlers ----

func (h *Handler) ListBankProfiles(_ context.Context, _ *connect.Request[floatv1.ListBankProfilesRequest]) (*connect.Response[floatv1.ListBankProfilesResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return connect.NewResponse(&floatv1.ListBankProfilesResponse{}), nil
	}
	out := make([]*floatv1.BankProfile, len(cfg.BankProfiles))
	for i, p := range cfg.BankProfiles {
		out[i] = &floatv1.BankProfile{Name: p.Name, RulesFile: p.RulesFile, SkipRules: p.SkipRules}
	}
	return connect.NewResponse(&floatv1.ListBankProfilesResponse{Profiles: out}), nil
}

func (h *Handler) CreateBankProfile(ctx context.Context, req *connect.Request[floatv1.CreateBankProfileRequest]) (*connect.Response[floatv1.CreateBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if req.Msg.RulesFile == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rules_file is required"))
	}

	// Reject path traversal attempts.
	cleaned := filepath.Clean(req.Msg.RulesFile)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rules_file must be a relative path within the data directory"))
	}

	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	// Check for duplicate name.
	for _, p := range cfg.BankProfiles {
		if p.Name == req.Msg.Name {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("bank profile %q already exists", req.Msg.Name))
		}
	}

	newProfile := config.BankProfile{Name: req.Msg.Name, RulesFile: cleaned, SkipRules: req.Msg.SkipRules}
	rulesFilePath := filepath.Join(h.dataDir, cleaned)
	createExtraPaths := []string{h.configPath}
	if len(req.Msg.RulesContent) > 0 {
		createExtraPaths = append(createExtraPaths, rulesFilePath)
	}
	err := h.lock.DoConfig(ctx, fmt.Sprintf("create bank profile %q", req.Msg.Name), createExtraPaths, func() error {
		// Write rules file if content provided.
		if len(req.Msg.RulesContent) > 0 {
			if err := os.MkdirAll(filepath.Dir(rulesFilePath), 0o755); err != nil {
				return fmt.Errorf("create rules dir: %w", err)
			}
			if err := os.WriteFile(rulesFilePath, req.Msg.RulesContent, 0o644); err != nil { //nolint:forbidigo // inside txlock.DoConfig
				return fmt.Errorf("write rules file: %w", err)
			}
		}

		// Append profile to config and save (copy-on-write).
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.BankProfiles = append(append([]config.BankProfile{}, cur.BankProfiles...), newProfile)
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "create bank profile failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "created bank profile", "name", req.Msg.Name, "rules_file", cleaned)
	return connect.NewResponse(&floatv1.CreateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: newProfile.Name, RulesFile: newProfile.RulesFile, SkipRules: newProfile.SkipRules},
	}), nil
}

func (h *Handler) GetBankProfileContent(ctx context.Context, req *connect.Request[floatv1.GetBankProfileContentRequest]) (*connect.Response[floatv1.GetBankProfileContentResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	profile, err := h.bankProfile(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rulesPath := filepath.Join(h.dataDir, profile.RulesFile)
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		if os.IsNotExist(err) {
			content = []byte{}
		} else {
			return nil, rpcErr(ctx, err, "read rules file failed")
		}
	}
	return connect.NewResponse(&floatv1.GetBankProfileContentResponse{
		RulesFile:    profile.RulesFile,
		RulesContent: content,
	}), nil
}

func (h *Handler) UpdateBankProfile(ctx context.Context, req *connect.Request[floatv1.UpdateBankProfileRequest]) (*connect.Response[floatv1.UpdateBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	newName := req.Msg.NewName
	if newName == "" {
		newName = req.Msg.Name
	}

	// Collect extra snapshot paths before acquiring the lock.
	preLock := h.loadCfg()
	extraPaths := []string{h.configPath}
	for _, p := range preLock.BankProfiles {
		if p.Name == req.Msg.Name && p.RulesFile != "" {
			extraPaths = append(extraPaths, filepath.Join(h.dataDir, p.RulesFile))
			break
		}
	}

	var updated config.BankProfile
	err := h.lock.DoConfig(ctx, fmt.Sprintf("update bank profile %q", req.Msg.Name), extraPaths, func() error {
		cur := h.loadCfg()
		profiles := make([]config.BankProfile, len(cur.BankProfiles))
		copy(profiles, cur.BankProfiles)

		// Check new name isn't already taken (unless it's the same profile).
		if newName != req.Msg.Name {
			for _, p := range profiles {
				if p.Name == newName {
					return fmt.Errorf("bank profile %q: %w", newName, journal.ErrAlreadyExists)
				}
			}
		}

		idx := -1
		for i, p := range profiles {
			if p.Name == req.Msg.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("bank profile %q: %w", req.Msg.Name, journal.ErrNotFound)
		}

		if len(req.Msg.RulesContent) > 0 {
			rulesPath := filepath.Join(h.dataDir, profiles[idx].RulesFile)
			if err := os.WriteFile(rulesPath, req.Msg.RulesContent, 0o644); err != nil { //nolint:forbidigo // inside txlock.DoConfig
				return fmt.Errorf("write rules file: %w", err)
			}
		}

		profiles[idx].Name = newName
		if req.Msg.SkipRules != nil {
			profiles[idx].SkipRules = *req.Msg.SkipRules
		}
		updated = profiles[idx]

		newCfg := *cur
		newCfg.BankProfiles = profiles
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, journal.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		return nil, rpcErr(ctx, err, "update bank profile failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated bank profile", "name", updated.Name)
	return connect.NewResponse(&floatv1.UpdateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: updated.Name, RulesFile: updated.RulesFile, SkipRules: updated.SkipRules},
	}), nil
}

func (h *Handler) DeleteBankProfile(ctx context.Context, req *connect.Request[floatv1.DeleteBankProfileRequest]) (*connect.Response[floatv1.DeleteBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	// Snapshot config.toml and the rules file (if it will be deleted).
	preLockDel := h.loadCfg()
	deleteExtraPaths := []string{h.configPath}
	for _, p := range preLockDel.BankProfiles {
		if p.Name == req.Msg.Name && req.Msg.DeleteRulesFile && p.RulesFile != "" {
			deleteExtraPaths = append(deleteExtraPaths, filepath.Join(h.dataDir, p.RulesFile))
			break
		}
	}

	err := h.lock.DoConfig(ctx, fmt.Sprintf("delete bank profile %q", req.Msg.Name), deleteExtraPaths, func() error {
		cur := h.loadCfg()
		idx := -1
		for i, p := range cur.BankProfiles {
			if p.Name == req.Msg.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("bank profile %q: %w", req.Msg.Name, journal.ErrNotFound)
		}

		rulesFile := cur.BankProfiles[idx].RulesFile
		newProfiles := make([]config.BankProfile, 0, len(cur.BankProfiles)-1)
		newProfiles = append(newProfiles, cur.BankProfiles[:idx]...)
		newProfiles = append(newProfiles, cur.BankProfiles[idx+1:]...)

		newCfg := *cur
		newCfg.BankProfiles = newProfiles
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)

		if req.Msg.DeleteRulesFile && rulesFile != "" {
			rulesPath := filepath.Join(h.dataDir, rulesFile)
			if err := os.Remove(rulesPath); err != nil && !os.IsNotExist(err) { //nolint:forbidigo // inside txlock.DoConfig
				slogctx.FromContext(ctx).WarnContext(ctx, "failed to delete rules file", "path", rulesPath, "error", err)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete bank profile failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "deleted bank profile", "name", req.Msg.Name)
	return connect.NewResponse(&floatv1.DeleteBankProfileResponse{}), nil
}

// profileToSlug converts a bank profile name to a URL/path-safe slug.
// e.g. "Chase Checking" → "chase-checking", "My Bank (US)" → "my-bank-us"
func profileToSlug(name string) string {
	var b strings.Builder
	prevHyphen := true // skip leading hyphens
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	s := b.String()
	return strings.TrimRight(s, "-")
}

// bankProfile finds a BankProfile by name in the config.
func (h *Handler) bankProfile(name string) (config.BankProfile, error) {
	cfg := h.loadCfg()
	if cfg != nil {
		for _, p := range cfg.BankProfiles {
			if p.Name == name {
				return p, nil
			}
		}
	}
	return config.BankProfile{}, fmt.Errorf("bank profile %q not found", name)
}
