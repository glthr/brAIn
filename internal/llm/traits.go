package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/glthr/brAIn/internal/model"
)

// stripCodeFences removes ```json ... ``` or ``` ... ``` wrapping from LLM output.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// NewInferProfileFunc returns an InferProfileFunc that uses the LLM to
// write or update social schema and behavioral priors from a user's memory footprint.
func NewInferProfileFunc(c *Client, logger *slog.Logger) model.InferProfileFunc {
	return func(ctx context.Context, userSummary string, current *model.ProfileInference) (*model.ProfileInference, error) {
		var sb strings.Builder
		sb.WriteString("/no_think\n")
		sb.WriteString(userSummary)

		if current != nil && (current.SocialSchema != "" || current.BehavioralPriors != "") {
			sb.WriteString("\n## Current profile\n")
			if current.SocialSchema != "" {
				sb.WriteString("Social Schema: ")
				sb.WriteString(current.SocialSchema)
				sb.WriteString("\n")
			}
			if current.BehavioralPriors != "" {
				sb.WriteString("Behavioral Priors: ")
				sb.WriteString(current.BehavioralPriors)
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\nAnalyze the above data and return the updated profile JSON.")

		reply, err := c.Chat(ctx, []Message{
			{Role: "system", Content: inferPersonalitySystemPrompt},
			{Role: "user", Content: sb.String()},
		}, 0.4)
		if err != nil {
			return nil, fmt.Errorf("infer profile: %w", err)
		}

		if logger != nil {
			logRawLLMResponse(ctx, logger, "infer_profile", reply)
		}

		cleaned := stripCodeFences(reply)

		var result model.ProfileInference
		if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
			// Log JSON parsing error with raw response
			if logger != nil {
				rawSanitized, sha, truncated := sanitizeRawLLM(reply)
				cleanedSanitized, cleanedTruncated := sanitizeAndTruncate(cleaned, 1000)
				logger.WarnContext(ctx, "failed to parse profile inference JSON, using fallback",
					"action", "infer_profile",
					"error", err,
					"raw_response", rawSanitized,
					"raw_response_sha256", sha,
					"raw_response_truncated", truncated,
					"cleaned_response", cleanedSanitized,
					"cleaned_response_truncated", cleanedTruncated)
			}
			// Fallback: if the LLM didn't return valid JSON, treat the
			// entire response as a social schema update.
			return &model.ProfileInference{
				SocialSchema: strings.TrimSpace(reply),
			}, nil
		}

		// Log the parsed result
		if logger != nil {
			logger.DebugContext(ctx, "parsed profile inference",
				"action", "infer_profile",
				"social_schema", result.SocialSchema,
				"behavioral_priors", result.BehavioralPriors,
				"social_schema_empty", result.SocialSchema == "",
				"behavioral_priors_empty", result.BehavioralPriors == "")
		}

		return &result, nil
	}
}
