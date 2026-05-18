package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/txlock"
)

// Migration is a one-time data transformation. Add new migrations to the
// canonical list in cmd/floatd/migrations.go. Never remove or reorder entries.
type Migration struct {
	ID          string
	Description string
	Run         func(ctx context.Context, dataDir string, hl *hledger.Client) error
}

type state struct {
	Applied []string `json:"applied"`
}

const stateFile = "migrations.json"

func loadState(dataDir string) (state, error) {
	path := filepath.Join(dataDir, stateFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("migrate: read state: %w", err)
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return state{}, fmt.Errorf("migrate: parse state: %w", err)
	}
	return s, nil
}

func saveState(dataDir string, s state) error {
	path := filepath.Join(dataDir, stateFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("migrate: marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("migrate: write state: %w", err)
	}
	return nil
}

// RunAll runs any pending migrations from the ordered list. Each migration runs
// inside its own txlock.Do() call (producing one git snapshot per migration).
// migrations.json is written after each successful migration and committed to
// git after all pending migrations complete.
func RunAll(ctx context.Context, migrations []Migration, lock *txlock.TxLock, snap *gitsnap.Repo, dataDir string, hl *hledger.Client) error {
	s, err := loadState(dataDir)
	if err != nil {
		return err
	}

	applied := make(map[string]bool, len(s.Applied))
	for _, id := range s.Applied {
		applied[id] = true
	}

	var ran bool
	for _, m := range migrations {
		if applied[m.ID] {
			slog.Debug("migrate: skipping already-applied migration", "id", m.ID)
			continue
		}

		slog.Info("migrate: running migration", "id", m.ID, "description", m.Description)

		if err := lock.Do(ctx, "migrate: "+m.ID, func() error {
			return m.Run(ctx, dataDir, hl)
		}); err != nil {
			return fmt.Errorf("migrate: %s: %w", m.ID, err)
		}

		s.Applied = append(s.Applied, m.ID)
		if err := saveState(dataDir, s); err != nil {
			return err
		}

		slog.Info("migrate: migration applied", "id", m.ID)
		ran = true
	}

	if ran && snap != nil {
		if err := snap.Commit(ctx, "migrate: record applied migrations"); err != nil {
			slog.Warn("migrate: gitsnap commit failed", "error", err)
		}
	}

	return nil
}
