package configstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
)

// ReasoningEffortCurationRequest is the write payload for a curated row.
type ReasoningEffortCurationRequest struct {
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	Aliases       []string          `json:"aliases,omitempty"`
	UpstreamField string            `json:"upstream_field"`
	Levels        []string          `json:"levels"`
	LevelMap      map[string]string `json:"level_map"`
}

// ValidateCuratedEffortRequest runs in the write handler before storage.
func ValidateCuratedEffortRequest(req ReasoningEffortCurationRequest) error {
	return schemas.ValidateCuratedEffortRow(schemas.CuratedEffortRow{
		Provider: req.Provider, Model: req.Model, Aliases: req.Aliases,
		UpstreamField: req.UpstreamField, Levels: req.Levels, LevelMap: req.LevelMap,
	})
}

func (req ReasoningEffortCurationRequest) toTable() (*tables.TableReasoningEffortCuration, error) {
	aliases, err := json.Marshal(req.Aliases)
	if err != nil {
		return nil, err
	}
	levels, err := json.Marshal(req.Levels)
	if err != nil {
		return nil, err
	}
	mapping, err := json.Marshal(req.LevelMap)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &tables.TableReasoningEffortCuration{
		Provider: req.Provider, Model: req.Model,
		AliasesJSON: string(aliases), UpstreamField: req.UpstreamField,
		LevelsJSON: string(levels), LevelMapJSON: string(mapping),
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func fromCuratedTable(row tables.TableReasoningEffortCuration) (ReasoningEffortCurationRequest, error) {
	out := ReasoningEffortCurationRequest{
		Provider: row.Provider, Model: row.Model, UpstreamField: row.UpstreamField,
	}
	if row.AliasesJSON != "" {
		if err := json.Unmarshal([]byte(row.AliasesJSON), &out.Aliases); err != nil {
			return out, err
		}
	}
	if err := json.Unmarshal([]byte(row.LevelsJSON), &out.Levels); err != nil {
		return out, err
	}
	if row.LevelMapJSON != "" {
		if err := json.Unmarshal([]byte(row.LevelMapJSON), &out.LevelMap); err != nil {
			return out, err
		}
	}
	return out, nil
}

// PutCuratedEffort validates and upserts one row, then installs it in the
// in-memory registry so the change applies without a restart.
func (s *RDBConfigStore) PutCuratedEffort(ctx context.Context, req ReasoningEffortCurationRequest) error {
	if err := ValidateCuratedEffortRequest(req); err != nil {
		return err
	}
	row, err := req.toTable()
	if err != nil {
		return err
	}
	db := s.DB().WithContext(ctx)
	if err := db.Save(row).Error; err != nil {
		return fmt.Errorf("failed to store curated effort row: %w", err)
	}
	return schemas.RegisterCuratedEffort(schemas.CuratedEffortRow{
		Provider: req.Provider, Model: req.Model, Aliases: req.Aliases,
		UpstreamField: req.UpstreamField, Levels: req.Levels, LevelMap: req.LevelMap,
	})
}

// ListCuratedEfforts returns every stored row.
func (s *RDBConfigStore) ListCuratedEfforts(ctx context.Context) ([]ReasoningEffortCurationRequest, error) {
	var rows []tables.TableReasoningEffortCuration
	if err := s.DB().WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ReasoningEffortCurationRequest, 0, len(rows))
	for _, row := range rows {
		req, err := fromCuratedTable(row)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

// LoadCuratedEfforts installs all stored rows in the in-memory registry at
// gateway start, before any request is served.
func (s *RDBConfigStore) LoadCuratedEfforts(ctx context.Context) error {
	rows, err := s.ListCuratedEfforts(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 && s.logger != nil {
		s.logger.Warn("reasoning effort curation table is empty; effort-bearing requests will be refused until rows are written")
	}
	schemas.ResetCuratedEfforts()
	for _, req := range rows {
		if err := schemas.RegisterCuratedEffort(schemas.CuratedEffortRow{
			Provider: req.Provider, Model: req.Model, Aliases: req.Aliases,
			UpstreamField: req.UpstreamField, Levels: req.Levels, LevelMap: req.LevelMap,
		}); err != nil {
			return fmt.Errorf("bad stored curated row %s/%s: %w", req.Provider, req.Model, err)
		}
	}
	return nil
}
