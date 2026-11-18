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

func chatRequestWithoutReasoning(reasoning *schemas.ChatReasoning) *schemas.BifrostChatRequest {
	return &schemas.BifrostChatRequest{
		Provider: "nous-custom",
		Model:    "tencent/hy4-flash:free",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}}},
		Params:   &schemas.ChatParameters{Reasoning: reasoning},
	}
}

func responsesRequestWithoutReasoning(provider schemas.ModelProvider, model string) *schemas.BifrostResponsesRequest {
	return &schemas.BifrostResponsesRequest{
		Provider: provider,
		Model:    model,
		Input: []schemas.ResponsesMessage{{
			Role:    schemas.Ptr(schemas.ResponsesInputMessageRoleUser),
			Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr("hi")},
		}},
		Params: &schemas.ResponsesParameters{},
	}
}

func TestCuratedNoEffortDefaultsToNearestHigh(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	if err := schemas.RegisterCuratedEffort(curatedRow()); err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)

	for _, tc := range []struct {
		name      string
		reasoning *schemas.ChatReasoning
	}{
		{name: "nil reasoning"},
		{name: "empty reasoning", reasoning: &schemas.ChatReasoning{}},
	} {
		t.Run("chat/"+tc.name, func(t *testing.T) {
			out := ToOpenAIChatRequest(ctx, chatRequestWithoutReasoning(tc.reasoning))
			if out == nil || out.ChatParameters.Reasoning == nil || out.ChatParameters.Reasoning.Effort == nil {
				t.Fatalf("default effort missing on wire: %+v", out)
			}
			if got := *out.ChatParameters.Reasoning.Effort; got != "high" {
				t.Fatalf("no-effort default must be nearest high, got %s", got)
			}
		})
	}

	t.Run("responses", func(t *testing.T) {
		out := ToOpenAIResponsesRequest(ctx, responsesRequestWithoutReasoning("nous-custom", "tencent/hy4-flash:free"))
		if out == nil || out.ResponsesParameters.Reasoning == nil || out.ResponsesParameters.Reasoning.Effort == nil {
			t.Fatalf("default effort missing on responses wire: %+v", out)
		}
		if got := *out.ResponsesParameters.Reasoning.Effort; got != "high" {
			t.Fatalf("no-effort default must be nearest high, got %s", got)
		}
	})
}

func TestCuratedBudgetOmittedLevelNeverReachesWire(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	row := curatedRow()
	row.Levels = []string{"none", "high", "max"}
	row.LevelMap = map[string]string{"none": "none", "high": "high", "max": "high"}
	if err := schemas.RegisterCuratedEffort(row); err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)

	chat := ToOpenAIChatRequest(ctx, &schemas.BifrostChatRequest{
		Provider: "nous-custom",
		Model:    "tencent/hy4-flash:free",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}}},
		Params:   &schemas.ChatParameters{Reasoning: &schemas.ChatReasoning{MaxTokens: schemas.Ptr(1000)}},
	})
	if chat == nil || (chat.ChatParameters.Reasoning != nil && chat.ChatParameters.Reasoning.Effort != nil) {
		t.Fatalf("unlisted budget level reached the chat wire: %+v", chat)
	}

	resp := ToOpenAIResponsesRequest(ctx, &schemas.BifrostResponsesRequest{
		Provider: "nous-custom",
		Model:    "tencent/hy4-flash:free",
		Input: []schemas.ResponsesMessage{{
			Role:    schemas.Ptr(schemas.ResponsesInputMessageRoleUser),
			Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr("hi")},
		}},
		Params: &schemas.ResponsesParameters{
			Reasoning: &schemas.ResponsesParametersReasoning{MaxTokens: schemas.Ptr(1000)},
		},
	})
	if resp == nil || (resp.ResponsesParameters.Reasoning != nil && resp.ResponsesParameters.Reasoning.Effort != nil) {
		t.Fatalf("unlisted budget level reached the responses wire: %+v", resp)
	}
}

func TestUncuratedNoEffortGetsNoDefault(t *testing.T) {
	schemas.ResetCuratedEfforts()
	t.Cleanup(schemas.ResetCuratedEfforts)
	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)

	chat := ToOpenAIChatRequest(ctx, &schemas.BifrostChatRequest{
		Provider: "nous-custom",
		Model:    "plain-model",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}}},
		Params:   &schemas.ChatParameters{},
	})
	if chat == nil || (chat.ChatParameters.Reasoning != nil && chat.ChatParameters.Reasoning.Effort != nil) {
		t.Fatalf("uncurated non-reasoning model must not get a default effort: %+v", chat)
	}

	responses := ToOpenAIResponsesRequest(ctx, responsesRequestWithoutReasoning("nous-custom", "plain-model"))
	if responses == nil || (responses.ResponsesParameters.Reasoning != nil && responses.ResponsesParameters.Reasoning.Effort != nil) {
		t.Fatalf("uncurated non-reasoning model must not get a default effort: %+v", responses)
	}
}
