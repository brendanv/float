package ledger

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/brendanv/float/internal/cube"
	"github.com/brendanv/float/internal/slogctx"
)

// cubePathPrefix is the mux prefix the cube is served under. The generation is
// carried in the path rather than a query parameter so the URL is
// content-addressed: a new generation is a new URL, which lets the response be
// cached immutably and makes a stale cube unreachable rather than merely
// unlikely.
const cubePathPrefix = "/api/cube/"

// cubeMaxAge is the immutable cache lifetime, in seconds. Safe because the URL
// changes whenever the content does.
const cubeMaxAge = 31536000

// GenerationHeader carries the current txlock generation on every Connect
// response, so any ordinary RPC keeps the client's view of the generation
// fresh without a dedicated poll.
const GenerationHeader = "X-Float-Generation"

// cubePayload is one encoded cube, held both raw and gzipped so a repeat
// request does not recompress.
type cubePayload struct {
	raw     []byte
	gzipped []byte
}

// cubeKey namespaces the cached payload by the config that affects bucketing
// and valuation. The cache is already generation-tiered, so the key only needs
// to cover what changes *without* bumping the generation — a timezone or
// reporting-currency change would otherwise serve a stale cube indefinitely.
func cubeKey(configHash string) string {
	return "cube:" + configHash
}

// cubeConfigHash derives the config fingerprint for the current settings.
// The config snapshot may legitimately be nil (handlers constructed without
// one), which is treated the same as an unset timezone.
func (h *Handler) cubeConfigHash() string {
	var timezone string
	if cfg := h.loadCfg(); cfg != nil {
		timezone = cfg.Timezone
	}
	return cube.ConfigHash(timezone, cube.DefaultReportingCurrency)
}

// buildCubePayload builds and encodes the cube for the current generation,
// deduplicated by the generation-tiered cache. It runs hledger and must never
// be called from inside txlock.Do.
func (h *Handler) buildCubePayload(ctx context.Context) (*cubePayload, error) {
	configHash := h.cubeConfigHash()
	return cachedGet(ctx, h.cache, cubeKey(configHash), func(ctx context.Context) (*cubePayload, error) {
		start := time.Now()
		c, err := cube.Build(ctx, h.hl, cube.Options{
			Generation: h.lock.Generation(),
			ConfigHash: configHash,
		})
		if err != nil {
			return nil, err
		}
		raw, err := cube.Encode(c)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
		if err != nil {
			return nil, err
		}
		if _, err := zw.Write(raw); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		slogctx.FromContext(ctx).InfoContext(ctx, "cube built",
			"generation", c.Generation,
			"postings", c.Postings.Len(),
			"accounts", c.Accounts.Len(),
			"bytes", len(raw),
			"gzipped", buf.Len(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return &cubePayload{raw: raw, gzipped: buf.Bytes()}, nil
	})
}

// CubeHandler serves GET /api/cube/{generation}.bin.
//
// It must be wrapped in auth.RequireAuth: the payload is the user's entire
// ledger. The static web assets are served without auth so the login page can
// render, so being an HTTP endpoint on floatd confers no protection.
func (h *Handler) CubeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, cubePathPrefix)
		genStr, ok := strings.CutSuffix(name, ".bin")
		if !ok || genStr == "" {
			http.Error(w, "expected /api/cube/{generation}.bin", http.StatusNotFound)
			return
		}
		requested, err := strconv.ParseUint(genStr, 10, 64)
		if err != nil {
			http.Error(w, "generation must be a number", http.StatusBadRequest)
			return
		}

		// Serving a generation other than the current one would hand the client
		// a cube it will cache immutably under a URL that promises otherwise.
		// Report the current generation instead and let the client refetch.
		current := h.lock.Generation()
		if requested != current {
			w.Header().Set(GenerationHeader, strconv.FormatUint(current, 10))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":      "generation is stale",
				"generation": current,
			})
			return
		}

		payload, err := h.buildCubePayload(r.Context())
		if err != nil {
			slogctx.FromContext(r.Context()).ErrorContext(r.Context(), "cube build failed", "error", err)
			http.Error(w, "cube build failed", http.StatusInternalServerError)
			return
		}

		// The generation is re-read after the build: if a write landed while
		// hledger was running, the payload describes a generation that is no
		// longer current and must not be cached under this URL.
		if now := h.lock.Generation(); now != requested {
			w.Header().Set(GenerationHeader, strconv.FormatUint(now, 10))
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "generation advanced during build", http.StatusConflict)
			return
		}

		body := payload.raw
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", cubeMaxAge))
		w.Header().Set(GenerationHeader, strconv.FormatUint(current, 10))
		w.Header().Set("Vary", "Accept-Encoding")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			body = payload.gzipped
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	})
}

// GenerationHandler serves GET /api/generation, which the web client calls once
// on load to learn which cube URL to fetch. Every subsequent RPC refreshes the
// value through GenerationHeader.
func (h *Handler) GenerationHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"generation": h.lock.Generation()})
	})
}

// WarmCube builds the cube in the background so the first dashboard load after
// a restart does not pay for it. Failures are logged and otherwise ignored: the
// lazy path in CubeHandler is the guarantee, this is only an optimization.
//
// Writes deliberately do not trigger a rebuild. Keeping the build off the write
// path is a hard requirement — it takes seconds, and a cube failure must not be
// able to fail or revert a journal write — so a post-write cube is rebuilt
// lazily on the next request, where singleflight collapses concurrent callers.
func (h *Handler) WarmCube(ctx context.Context) {
	logger := slogctx.FromContext(ctx)
	if _, err := h.buildCubePayload(ctx); err != nil {
		logger.WarnContext(ctx, "cube warm-up failed; it will be built on first request", "error", err)
	}
}
