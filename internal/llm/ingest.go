package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/glthr/brAIn/internal/model"
	"github.com/tidwall/gjson"
)

// encodingTimeout is the maximum time allowed for a single LLM encoding call.
const encodingTimeout = 20 * time.Minute

// NewEncoderFunc returns an EncoderFunc that uses the LLM to extract structured
// memories from a single conversation turn (user prompt + agent response).
func NewEncoderFunc(c *Client) model.EncoderFunc {
	return func(ctx context.Context, userPrompt, agentResponse string) (*model.EncodingResult, error) {
		callCtx, cancel := context.WithTimeout(ctx, encodingTimeout)
		defer cancel()

		var sb strings.Builder
		sb.WriteString("/no_think\n## User prompt\n")
		sb.WriteString(userPrompt)
		sb.WriteString("\n\n## Agent response\n")
		sb.WriteString(agentResponse)

		reply, err := c.Chat(callCtx, []Message{
			{Role: "system", Content: ingestSystemPrompt},
			{Role: "user", Content: sb.String()},
		}, 0.3)
		if err != nil {
			return nil, fmt.Errorf("encode: %w", err)
		}

		if logger := loggerFromContext(callCtx); logger != nil {
			logRawLLMResponse(callCtx, logger, "encode", reply)
		}

		reply = strings.TrimSpace(reply)
		reply = stripCodeFences(reply)

		if reply == "" {
			return &model.EncodingResult{
				Episode: "Empty conversation turn",
				Outcome: "partial",
				Valence: 0.5,
			}, nil
		}

		var result model.EncodingResult
		if err := json.Unmarshal([]byte(reply), &result); err != nil {
			if logger := loggerFromContext(callCtx); logger != nil {
				sanitized, sha, truncated := sanitizeRawLLM(reply)
				logger.WarnContext(callCtx, "failed to parse encoding JSON",
					"action", "encode",
					"error", err,
					"raw_response", sanitized,
					"raw_response_sha256", sha,
					"raw_response_truncated", truncated)
			}
			trimmed, _ := sanitizeAndTruncate(reply, 300)
			return nil, fmt.Errorf("encode: parse JSON: %w (raw: %s)", err, trimmed)
		}

		// Normalize outcome.
		switch result.Outcome {
		case "success", "partial", "failure":
		default:
			result.Outcome = "partial"
		}

		// Clamp valence.
		if result.Valence < -1 {
			result.Valence = -1
		}
		if result.Valence > 1 {
			result.Valence = 1
		}

		return &result, nil
	}
}

// NewBatchEncoderFunc returns a BatchEncoderFunc that sends all turns in one LLM call
// and parses a JSON array of EncodingResult. The LLM may merge similar turns, so the
// array may have 1 to N elements (N = len(turns)); the pipeline consumes all turns
// and stores the returned memories.
func NewBatchEncoderFunc(c *Client) model.BatchEncoderFunc {
	return func(ctx context.Context, turns []model.EncodingTurn) ([]model.EncodingResult, error) {
		if len(turns) == 0 {
			return nil, nil
		}
		callCtx, cancel := context.WithTimeout(ctx, encodingTimeout)
		defer cancel()

		var sb strings.Builder
		sb.WriteString("/no_think\n\n")
		for i, t := range turns {
			fmt.Fprintf(&sb, "## Turn %d\n### User prompt\n%s\n### Agent response\n%s\n\n", i+1, t.UserPrompt, t.AgentResponse)
		}
		userContent := sb.String()
		if logger := loggerFromContext(callCtx); logger != nil {
			logger.InfoContext(callCtx, "batch encode request", "action", "batch_encode", "turns", len(turns), "user_content_bytes", len(userContent), "system_prompt_bytes", len(ingestBatchSystemPrompt))
		}

		// Limit output tokens so the model stops as soon as the JSON array is complete (faster).
		llmStart := time.Now()
		reply, err := c.ChatWithMaxTokens(callCtx, []Message{
			{Role: "system", Content: ingestBatchSystemPrompt},
			{Role: "user", Content: userContent},
		}, 0.3, 512)
		if logger := loggerFromContext(callCtx); logger != nil {
			logger.InfoContext(callCtx, "batch encode LLM response", "action", "batch_encode", "duration_s", time.Since(llmStart).Seconds(), "reply_bytes", len(reply), "error", err)
		}
		if err != nil {
			return nil, fmt.Errorf("batch encode: %w", err)
		}

		if logger := loggerFromContext(callCtx); logger != nil {
			logRawLLMResponse(callCtx, logger, "batch_encode", reply)
		}

		reply = strings.TrimSpace(reply)
		reply = stripCodeFences(reply)

		if reply == "" {
			// Single default result (treat all turns as one merged empty)
			return []model.EncodingResult{{Episode: "Empty conversation turn", Outcome: "partial", Valence: 0.5}}, nil
		}

		parsed := gjson.Parse(reply)
		arr := parsed.Array()
		if len(arr) == 0 {
			// Root might be a single object (LLM returned one merged result)
			if parsed.Exists() && parsed.Type == gjson.JSON {
				arr = []gjson.Result{parsed}
			}
		}
		results := make([]model.EncodingResult, 0, len(arr))
		for _, elem := range arr {
			var one model.EncodingResult
			if err := json.Unmarshal([]byte(elem.Raw), &one); err != nil {
				if logger := loggerFromContext(callCtx); logger != nil {
					sanitized, sha, truncated := sanitizeRawLLM(reply)
					logger.WarnContext(callCtx, "failed to parse batch encoding JSON",
						"action", "batch_encode",
						"error", err,
						"raw_response", sanitized,
						"raw_response_sha256", sha,
						"raw_response_truncated", truncated)
				}
				trimmed, _ := sanitizeAndTruncate(reply, 300)
				return nil, fmt.Errorf("batch encode: parse JSON: %w (raw: %s)", err, trimmed)
			}
			results = append(results, one)
		}

		if len(results) == 0 {
			return nil, fmt.Errorf("batch encode: got 0 results for %d turns (need at least 1)", len(turns))
		}

		for i := range results {
			switch results[i].Outcome {
			case "success", "partial", "failure":
			default:
				results[i].Outcome = "partial"
			}
			if results[i].Valence < -1 {
				results[i].Valence = -1
			}
			if results[i].Valence > 1 {
				results[i].Valence = 1
			}
		}
		return results, nil
	}
}
