package journal

import (
	"context"
	"fmt"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// ModifyTags replaces all transaction-level user-visible tags on the transaction
// identified by fid. tags is the complete desired set of non-float- tags.
// The free-text comment, hidden meta, status, and postings are preserved unchanged.
// Callers must wrap in txlock.Do().
func ModifyTags(ctx context.Context, client *hledger.Client, dataDir, fid string, tags map[string]string) error {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return fmt.Errorf("journal: modify-tags: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: modify-tags: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: modify-tags: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	t := txns[0]
	src := &SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := inputFromTransaction(t)
	if err != nil {
		return fmt.Errorf("journal: modify-tags: %w", err)
	}
	input.Tags = tags

	if _, err := WriteTransaction(ctx, client, dataDir, input, src); err != nil {
		return fmt.Errorf("journal: modify-tags: write: %w", err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction tags modified", "fid", fid)
	return nil
}

// ModifyFloatMeta replaces all hidden-meta tags (those with hledger.HiddenMetaPrefix) on the
// transaction identified by fid. meta is the complete desired set; keys must include the
// "float-" prefix. User-visible tags and free-text comments are preserved unchanged.
// Callers must wrap in txlock.Do().
func ModifyFloatMeta(ctx context.Context, client *hledger.Client, dataDir, fid string, meta map[string]string) error {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: modify-hidden-meta: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: modify-hidden-meta: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	t := txns[0]
	src := &SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := inputFromTransaction(t)
	if err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: %w", err)
	}
	input.FloatMeta = meta

	if _, err := WriteTransaction(ctx, client, dataDir, input, src); err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: write: %w", err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction hidden meta modified", "fid", fid)
	return nil
}
