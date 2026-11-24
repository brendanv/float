package ledger

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/templates"
)

// ---- Template handlers ----

func toProtoTemplate(t templates.Template) *floatv1.TransactionTemplate {
	postings := make([]*floatv1.TemplatePosting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = &floatv1.TemplatePosting{
			Account:         p.Account,
			Commodity:       p.Commodity,
			DefaultQuantity: p.DefaultQuantity,
			Comment:         p.Comment,
		}
	}
	return &floatv1.TransactionTemplate{
		Id:       t.ID,
		Name:     t.Name,
		Payee:    t.Payee,
		Note:     t.Note,
		Postings: postings,
		Tags:     t.Tags,
	}
}

func (h *Handler) ListTemplates(ctx context.Context, _ *connect.Request[floatv1.ListTemplatesRequest]) (*connect.Response[floatv1.ListTemplatesResponse], error) {
	ts, err := templates.Load(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "list templates failed")
	}
	out := make([]*floatv1.TransactionTemplate, len(ts))
	for i, t := range ts {
		out[i] = toProtoTemplate(t)
	}
	return connect.NewResponse(&floatv1.ListTemplatesResponse{Templates: out}), nil
}

func (h *Handler) AddTemplate(ctx context.Context, req *connect.Request[floatv1.AddTemplateRequest]) (*connect.Response[floatv1.TransactionTemplate], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for _, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("each posting must have an account"))
		}
	}

	var added templates.Template
	err := h.lock.DoConfig(ctx, fmt.Sprintf("add template: %s", req.Msg.Name), []string{templates.FilePath(h.dataDir)}, func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		postings := make([]templates.TemplatePosting, len(req.Msg.Postings))
		for i, p := range req.Msg.Postings {
			postings[i] = templates.TemplatePosting{
				Account:         p.Account,
				Commodity:       p.Commodity,
				DefaultQuantity: p.DefaultQuantity,
				Comment:         p.Comment,
			}
		}
		added = templates.Template{
			ID:       journal.MintFID(),
			Name:     req.Msg.Name,
			Payee:    req.Msg.Payee,
			Note:     req.Msg.Note,
			Postings: postings,
			Tags:     req.Msg.Tags,
		}
		ts = append(ts, added)
		return templates.Save(h.dataDir, ts)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add template failed")
	}
	return connect.NewResponse(toProtoTemplate(added)), nil
}

func (h *Handler) UpdateTemplate(ctx context.Context, req *connect.Request[floatv1.UpdateTemplateRequest]) (*connect.Response[floatv1.TransactionTemplate], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for _, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("each posting must have an account"))
		}
	}

	var updated templates.Template
	err := h.lock.DoConfig(ctx, fmt.Sprintf("update template %s", req.Msg.Id), []string{templates.FilePath(h.dataDir)}, func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		found := false
		for i, t := range ts {
			if t.ID == req.Msg.Id {
				postings := make([]templates.TemplatePosting, len(req.Msg.Postings))
				for j, p := range req.Msg.Postings {
					postings[j] = templates.TemplatePosting{
						Account:         p.Account,
						Commodity:       p.Commodity,
						DefaultQuantity: p.DefaultQuantity,
						Comment:         p.Comment,
					}
				}
				ts[i] = templates.Template{
					ID:       req.Msg.Id,
					Name:     req.Msg.Name,
					Payee:    req.Msg.Payee,
					Note:     req.Msg.Note,
					Postings: postings,
					Tags:     req.Msg.Tags,
				}
				updated = ts[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("template %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return templates.Save(h.dataDir, ts)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update template failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(toProtoTemplate(updated)), nil
}

func (h *Handler) DeleteTemplate(ctx context.Context, req *connect.Request[floatv1.DeleteTemplateRequest]) (*connect.Response[floatv1.DeleteTemplateResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	err := h.lock.DoConfig(ctx, fmt.Sprintf("delete template %s", req.Msg.Id), []string{templates.FilePath(h.dataDir)}, func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		filtered := ts[:0]
		found := false
		for _, t := range ts {
			if t.ID == req.Msg.Id {
				found = true
				continue
			}
			filtered = append(filtered, t)
		}
		if !found {
			return fmt.Errorf("template %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return templates.Save(h.dataDir, filtered)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete template failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.DeleteTemplateResponse{}), nil
}
