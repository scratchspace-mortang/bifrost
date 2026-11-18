package openai

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func curatedRow() schemas.CuratedEffortRow {
	return schemas.CuratedEffortRow{
		Provider: "nous-custom", Model: "tencent/hy4-flash:free",
		UpstreamField: "reasoning_effort",
		Levels:        []string{"low", "medium", "high", "max"},
		LevelMap:      map[string]string{"low": "low", "medium": "medium", "high": "high", "max": "high"},
	}
}

func chatRequestWithReasoning(effort *string, budget *int) *schemas.BifrostChatRequest {
	return &schemas.BifrostChatRequest{
		Provider: "nous-custom",
		Model:    "tencent/hy4-flash:free",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}}},
		Params: &schemas.ChatParameters{
			MaxCompletionTokens: budget,
			Reasoning:           &schemas.ChatReasoning{Effort: effort, MaxTokens: budget},
		},
	}
}

func TestCuratedCallerLevelOnChatWire(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	if err := schemas.RegisterCuratedEffort(curatedRow()); err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	out := ToOpenAIChatRequest(ctx, chatRequestWithReasoning(schemas.Ptr("max"), nil))
	if out == nil || out.ChatParameters.Reasoning == nil || out.ChatParameters.Reasoning.Effort == nil {
		t.Fatalf("effort missing on wire: %+v", out)
	}
	if got := *out.ChatParameters.Reasoning.Effort; got != "high" {
		t.Fatalf("curated max must write high, got %s", got)
	}
}

func TestCuratedBudgetLevelOnChatWire(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	if err := schemas.RegisterCuratedEffort(curatedRow()); err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	// A large budget derives "high"; the curated map writes "high" unchanged.
	out := ToOpenAIChatRequest(ctx, chatRequestWithReasoning(nil, schemas.Ptr(1000)))
	if out == nil || out.ChatParameters.Reasoning == nil || out.ChatParameters.Reasoning.Effort == nil {
		t.Fatalf("derived effort missing on wire: %+v", out)
	}
	if got := *out.ChatParameters.Reasoning.Effort; got != "high" {
		t.Fatalf("curated budget level must write high, got %s", got)
	}
}

func TestCuratedBudgetUsesMapping(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	row := curatedRow()
	// Make the highest derived level map to a different upstream level so a
	// bypass of the curated path on the budget branch is detectable.
	row.LevelMap["high"] = "medium"
	if err := schemas.RegisterCuratedEffort(row); err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	out := ToOpenAIChatRequest(ctx, chatRequestWithReasoning(nil, schemas.Ptr(1000)))
	if out == nil || out.ChatParameters.Reasoning == nil || out.ChatParameters.Reasoning.Effort == nil {
		t.Fatalf("derived effort missing on wire: %+v", out)
	}
	if got := *out.ChatParameters.Reasoning.Effort; got != "medium" {
		t.Fatalf("budget-derived level bypassed curated mapping: %s", got)
	}
}
