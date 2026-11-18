package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/maximhq/bifrost/framework/configstore"
	"github.com/valyala/fasthttp"
)

type curatedEffortWriter interface {
	PutCuratedEffort(ctx context.Context, req configstore.ReasoningEffortCurationRequest) error
}

type curatedEffortLister interface {
	ListCuratedEfforts(ctx context.Context) ([]configstore.ReasoningEffortCurationRequest, error)
}

// putCuratedEffort handles POST /api/reasoning-effort-curation. Validation runs
// here, before the row is stored, so a typo never becomes a permanent level.
func (h *ConfigHandler) putCuratedEffort(ctx *fasthttp.RequestCtx) {
	store, ok := h.store.ConfigStore.(curatedEffortWriter)
	if !ok {
		SendError(ctx, http.StatusServiceUnavailable, "config store does not support curated efforts")
		return
	}
	var req configstore.ReasoningEffortCurationRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, http.StatusBadRequest, "invalid curated effort row: "+err.Error())
		return
	}
	if err := configstore.ValidateCuratedEffortRequest(req); err != nil {
		SendError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.PutCuratedEffort(ctx, req); err != nil {
		SendError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SendJSONWithStatus(ctx, map[string]string{"status": "stored", "provider": req.Provider, "model": req.Model}, http.StatusOK)
}

// listCuratedEfforts handles GET /api/reasoning-effort-curation.
func (h *ConfigHandler) listCuratedEfforts(ctx *fasthttp.RequestCtx) {
	store, ok := h.store.ConfigStore.(curatedEffortLister)
	if !ok {
		SendError(ctx, http.StatusServiceUnavailable, "config store does not support curated efforts")
		return
	}
	rows, err := store.ListCuratedEfforts(ctx)
	if err != nil {
		SendError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"rows": rows})
}
