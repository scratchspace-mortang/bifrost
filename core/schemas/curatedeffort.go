package schemas

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// CuratedEffortRow is a local, hand-curated reasoning-effort entry. It is keyed
// by provider and model only. Aliases name other model strings that resolve to
// the same row at ingress, before alias resolution runs in the provider.
type CuratedEffortRow struct {
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	Aliases       []string          `json:"aliases,omitempty"`
	UpstreamField string            `json:"upstream_field"`
	Levels        []string          `json:"levels"`
	LevelMap      map[string]string `json:"level_map"`
}

var curatedEffortKnown = map[string]bool{
	ReasoningEffortNone: true, ReasoningEffortMinimal: true,
	ReasoningEffortLow: true, ReasoningEffortMedium: true,
	ReasoningEffortHigh: true, ReasoningEffortXHigh: true,
	ReasoningEffortMax: true,
}

var curatedEffortRegistry struct {
	sync.RWMutex
	rows map[string]CuratedEffortRow
}

func curatedEffortKey(provider, model string) string {
	return provider + "\x00" + model
}

// ValidateCuratedEffortRow checks a row before it is stored. A row with no rung
// is refused: the default walk has no candidate and would send a level nobody
// chose. Every level and every mapped level must be a known label, and a mapped
// caller level must be one of the row's levels.
func ValidateCuratedEffortRow(row CuratedEffortRow) error {
	if strings.TrimSpace(row.Provider) == "" || strings.TrimSpace(row.Model) == "" {
		return fmt.Errorf("curated effort row needs a provider and a model")
	}
	if strings.TrimSpace(row.UpstreamField) == "" {
		return fmt.Errorf("curated effort row needs an upstream field name")
	}
	if len(row.Levels) == 0 {
		return fmt.Errorf("curated effort row needs at least one level")
	}
	rung := false
	for _, level := range row.Levels {
		if !curatedEffortKnown[level] {
			return fmt.Errorf("unknown effort level %q", level)
		}
		if level != ReasoningEffortNone {
			rung = true
		}
	}
	if !rung {
		return fmt.Errorf("curated effort row has no rung: a none-only ladder is refused")
	}
	for caller, upstream := range row.LevelMap {
		if !curatedEffortKnown[caller] || !curatedEffortKnown[upstream] {
			return fmt.Errorf("unknown effort level in map for %q", caller)
		}
		if !containsString(row.Levels, caller) {
			return fmt.Errorf("mapped caller level %q is not listed by the row", caller)
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// RegisterCuratedEffort validates and installs a row. The row is indexed by
// its provider and model and by each declared alias.
func RegisterCuratedEffort(row CuratedEffortRow) error {
	if err := ValidateCuratedEffortRow(row); err != nil {
		return err
	}
	curatedEffortRegistry.Lock()
	defer curatedEffortRegistry.Unlock()
	if curatedEffortRegistry.rows == nil {
		curatedEffortRegistry.rows = make(map[string]CuratedEffortRow)
	}
	curatedEffortRegistry.rows[curatedEffortKey(row.Provider, row.Model)] = row
	for _, alias := range row.Aliases {
		curatedEffortRegistry.rows[curatedEffortKey(row.Provider, alias)] = row
	}
	return nil
}

// ResetCuratedEfforts clears the registry; test setup uses it.
func ResetCuratedEfforts() {
	curatedEffortRegistry.Lock()
	defer curatedEffortRegistry.Unlock()
	curatedEffortRegistry.rows = make(map[string]CuratedEffortRow)
}

// CuratedEffortFor returns the row for a provider and model, if curated.
func CuratedEffortFor(provider, model string) (CuratedEffortRow, bool) {
	curatedEffortRegistry.RLock()
	defer curatedEffortRegistry.RUnlock()
	row, ok := curatedEffortRegistry.rows[curatedEffortKey(provider, model)]
	return row, ok
}

// AllCuratedEfforts returns a snapshot of the primary rows.
func AllCuratedEfforts() []CuratedEffortRow {
	curatedEffortRegistry.RLock()
	defer curatedEffortRegistry.RUnlock()
	out := make([]CuratedEffortRow, 0, len(curatedEffortRegistry.rows))
	seen := make(map[string]bool)
	for _, row := range curatedEffortRegistry.rows {
		k := curatedEffortKey(row.Provider, row.Model)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, row)
	}
	return out
}

// ApplyCuratedEffort maps a caller level to the upstream level the curated row
// names. It returns the input unchanged when the model is not curated. It is
// used on the caller path and on the budget-derived path so both write only a
// level the row declares.
func ApplyCuratedEffort(provider, model, effort string) string {
	row, ok := CuratedEffortFor(provider, model)
	if !ok {
		return effort
	}
	if mapped, exists := row.LevelMap[effort]; exists {
		return mapped
	}
	return effort
}

// CuratedUpstreamField returns the field name a curated row writes.
func CuratedUpstreamField(provider, model string) string {
	if row, ok := CuratedEffortFor(provider, model); ok {
		return row.UpstreamField
	}
	return "reasoning_effort"
}

// BudgetLevel mirrors the gateway's budget-to-effort conversion for the
// validation seam. The provider path still calls the canonical helper.
func BudgetLevel(budget int) string {
	if budget <= 0 {
		return ReasoningEffortNone
	}
	ratio := float64(budget) / 100.0
	switch {
	case ratio <= 0.25:
		return ReasoningEffortLow
	case ratio <= 0.60:
		return ReasoningEffortMedium
	default:
		return ReasoningEffortHigh
	}
}

// ValidateCuratedRequest applies the reject rules at ingress. A request with
// no effort and no budget is untouched. Any effort, including a derived one,
// requires a curated row and a level that row lists.
func ValidateCuratedRequest(provider, model string, effort *string, budget *int) error {
	given := effort != nil
	if !given && budget != nil && *budget > 0 {
		given = true
	}
	if !given {
		return nil
	}
	row, ok := CuratedEffortFor(provider, model)
	if !ok {
		return fmt.Errorf("model %s/%s is not curated: reasoning effort requires a curated entry", provider, model)
	}
	value := ""
	if effort != nil {
		value = *effort
	} else {
		value = BudgetLevel(*budget)
	}
	if _, listed := row.LevelMap[value]; !listed {
		levels := append([]string(nil), row.Levels...)
		sort.Strings(levels)
		return fmt.Errorf("level %q is not listed for %s; its levels are %s", value, model, strings.Join(levels, ", "))
	}
	return nil
}

// CuratedNearestHigh returns the row's nearest rung to high, ties upward. none
// is not a rung. It is used only for the default on a no-effort request.
func CuratedNearestHigh(provider, model string) (string, bool) {
	row, ok := CuratedEffortFor(provider, model)
	if !ok {
		return "", false
	}
	order := []string{ReasoningEffortMinimal, ReasoningEffortLow, ReasoningEffortMedium,
		ReasoningEffortHigh, ReasoningEffortXHigh, ReasoningEffortMax}
	best, bestDistance := "", 0
	target := 3 // high
	for rung, candidate := range order {
		if !containsString(row.Levels, candidate) {
			continue
		}
		distance := rung - target
		if distance < 0 {
			distance = -distance
		}
		if best == "" || distance < bestDistance || (distance == bestDistance && rung > target) {
			best, bestDistance = candidate, distance
		}
	}
	return best, best != ""
}
