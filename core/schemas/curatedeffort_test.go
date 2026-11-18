package schemas

import (
	"encoding/json"
	"strings"
	"testing"
)

func testRow() CuratedEffortRow {
	return CuratedEffortRow{
		Provider: "nous-custom", Model: "tencent/hy4-flash:free",
		Aliases:       []string{"hy4-nous-free"},
		UpstreamField: "reasoning_effort",
		Levels:        []string{"low", "medium", "high", "max"},
		LevelMap:      map[string]string{"low": "low", "medium": "medium", "high": "high", "max": "high"},
	}
}

func TestValidateCuratedEffortRow(t *testing.T) {
	ResetCuratedEfforts()
	t.Cleanup(ResetCuratedEfforts)
	if err := ValidateCuratedEffortRow(testRow()); err != nil {
		t.Fatalf("valid row rejected: %v", err)
	}
	noneOnly := testRow()
	noneOnly.Levels = []string{"none"}
	noneOnly.LevelMap = map[string]string{"none": "none"}
	if err := ValidateCuratedEffortRow(noneOnly); err == nil || !strings.Contains(err.Error(), "no rung") {
		t.Fatalf("none-only row accepted or wrong error: %v", err)
	}
	badField := testRow()
	badField.UpstreamField = "reasoning.effort"
	if err := ValidateCuratedEffortRow(badField); err == nil || !strings.Contains(err.Error(), "cannot be written") {
		t.Fatalf("unwritable upstream field accepted: %v", err)
	}
	partial := testRow()
	delete(partial.LevelMap, "high")
	if err := ValidateCuratedEffortRow(partial); err == nil || !strings.Contains(err.Error(), "no map entry") {
		t.Fatalf("listed-but-unmapped level accepted: %v", err)
	}
	badLevel := testRow()
	badLevel.Levels = []string{"ultra"}
	if err := ValidateCuratedEffortRow(badLevel); err == nil || !strings.Contains(err.Error(), "unknown effort level") {
		t.Fatalf("unknown level accepted or wrong error: %v", err)
	}
}

func TestValidateCuratedRequest(t *testing.T) {
	ResetCuratedEfforts()
	t.Cleanup(ResetCuratedEfforts)
	if err := RegisterCuratedEffort(testRow()); err != nil {
		t.Fatal(err)
	}
	effort := "max"
	if err := ValidateCuratedRequest("nous-custom", "hy4-nous-free", &effort, nil, ReasoningBudgetCeiling); err != nil {
		t.Fatalf("listed caller level refused: %v", err)
	}
	bad := "ultra"
	err := ValidateCuratedRequest("nous-custom", "hy4-nous-free", &bad, nil, ReasoningBudgetCeiling)
	if err == nil || !strings.Contains(err.Error(), `level "ultra" is not listed`) || !strings.Contains(err.Error(), "high") {
		t.Fatalf("unlisted level not refused with row levels: %v", err)
	}
	err = ValidateCuratedRequest("nous-custom", "other/model", &effort, nil, ReasoningBudgetCeiling)
	if err == nil || !strings.Contains(err.Error(), "not curated") {
		t.Fatalf("uncurated effort not refused: %v", err)
	}
	if err := ValidateCuratedRequest("nous-custom", "hy4-nous-free", nil, nil, ReasoningBudgetCeiling); err != nil {
		t.Fatalf("no-effort request refused: %v", err)
	}
	budget := 90
	if err := ValidateCuratedRequest("nous-custom", "hy4-nous-free", nil, &budget, ReasoningBudgetCeiling); err != nil {
		t.Fatalf("listed budget level refused: %v", err)
	}
}

func TestBudgetUsesOneConverterAtBothSeams(t *testing.T) {
	ResetCuratedEfforts()
	t.Cleanup(ResetCuratedEfforts)
	row := testRow()
	row.Levels = []string{"none", "high", "max"}
	row.LevelMap = map[string]string{"none": "none", "high": "high", "max": "high"}
	if err := RegisterCuratedEffort(row); err != nil {
		t.Fatal(err)
	}
	lowRow := testRow()
	lowRow.Model = "tencent/hy4-flash:low-listed"
	lowRow.Aliases = nil
	lowRow.Levels = []string{"none", "low", "high", "max"}
	lowRow.LevelMap = map[string]string{"none": "none", "low": "low", "high": "high", "max": "high"}
	if err := RegisterCuratedEffort(lowRow); err != nil {
		t.Fatal(err)
	}

	// The canonical ratio for 1000 over the 4096 default is low. A row that
	// omits low must refuse this budget, not accept it at ingress and write low.
	budget := 1000
	if err := ValidateCuratedRequest("nous-custom", "tencent/hy4-flash:free", nil, &budget, ReasoningBudgetCeiling); err == nil {
		t.Fatal("unlisted canonical budget level was accepted")
	}
	if got := ReasoningLevelFromBudget(budget, ReasoningBudgetFloor, ReasoningBudgetCeiling); got != ReasoningEffortLow {
		t.Fatalf("canonical budget level changed: %s", got)
	}

	// 50 used to be refused as medium by the old fixed-100 converter; the
	// canonical ratio makes it low, which this row lists.
	small := 50
	if err := ValidateCuratedRequest("nous-custom", lowRow.Model, nil, &small, ReasoningBudgetCeiling); err != nil {
		t.Fatalf("listed canonical budget level refused: %v", err)
	}
}

func TestApplyCuratedEffort(t *testing.T) {
	ResetCuratedEfforts()
	t.Cleanup(ResetCuratedEfforts)
	if got := ApplyCuratedEffort("nous-custom", "unknown", "max"); got != "max" {
		t.Fatalf("uncurated model changed: %s", got)
	}
	if err := RegisterCuratedEffort(testRow()); err != nil {
		t.Fatal(err)
	}
	if got := ApplyCuratedEffort("nous-custom", "tencent/hy4-flash:free", "max"); got != "high" {
		t.Fatalf("curated max must map to observed high, got %s", got)
	}
	if got := ApplyCuratedEffort("nous-custom", "hy4-nous-free", "max"); got != "high" {
		t.Fatalf("alias must resolve to the same row, got %s", got)
	}
}

func TestFiveEffortNamesFoldInChat(t *testing.T) {
	ResetCuratedEfforts()
	t.Cleanup(ResetCuratedEfforts)
	for _, name := range []string{"reasoning_effort", "effort", "thinking.level", "thinking_level"} {
		var cp ChatParameters
		payload := `{"` + name + `":"max"}`
		if err := json.Unmarshal([]byte(payload), &cp); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cp.Reasoning == nil || cp.Reasoning.Effort == nil || *cp.Reasoning.Effort != "max" {
			t.Fatalf("%s did not fold: %+v", name, cp.Reasoning)
		}
	}
	var cp ChatParameters
	payload := `{"reasoning":{"effort":"high"},"thinking_level":"low"}`
	err := json.Unmarshal([]byte(payload), &cp)
	if err == nil || !strings.Contains(err.Error(), "reasoning.effort") || !strings.Contains(err.Error(), "thinking_level") {
		t.Fatalf("conflict not refused with both field names: %v", err)
	}
	var same ChatParameters
	if err := json.Unmarshal([]byte(`{"effort":"high","thinking_level":"high"}`), &same); err != nil {
		t.Fatalf("same value in two names refused: %v", err)
	}
}
