package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/glthr/brAIn/internal/model"
)

const commonPrefix = "COMMON:"
const emergingSkillPrefix = "EMERGING_SKILL:"

// EmergedSkillStorer is called when consolidation output contains an EMERGING_SKILL line.
// The implementer should store the procedure (e.g. in procedural memory).
type EmergedSkillStorer interface {
	StoreEmergedSkill(ctx context.Context, goal string, steps []string) error
}

// emergingSkillLine matches "EMERGING_SKILL: goal (step1; step2; ...)".
// Capturing groups: goal, steps-in-parens.
var emergingSkillLine = regexp.MustCompile(`(?m)^EMERGING_SKILL:\s*(.+?)\s*\(([^)]*)\)\s*$`)

// parseAndStoreEmergedSkills extracts EMERGING_SKILL lines from reply, calls storer for each,
// and returns the reply with those lines removed (and trimmed).
func parseAndStoreEmergedSkills(ctx context.Context, reply string, storer EmergedSkillStorer) string {
	if storer == nil {
		return reply
	}
	var out []string
	for _, line := range strings.Split(reply, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), emergingSkillPrefix) {
			out = append(out, line)
			continue
		}
		matches := emergingSkillLine.FindStringSubmatch(trimmed)
		if len(matches) != 3 {
			out = append(out, line)
			continue
		}
		goal := strings.TrimSpace(matches[1])
		stepsStr := strings.TrimSpace(matches[2])
		var steps []string
		for _, s := range strings.Split(stepsStr, ";") {
			s = strings.TrimSpace(s)
			if s != "" {
				steps = append(steps, s)
			}
		}
		if goal != "" && len(steps) > 0 {
			_ = storer.StoreEmergedSkill(ctx, goal, steps)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// NewSemanticizationFunc returns a SemanticizationFunc that uses the LLM to compress
// old episodic memories into a single semantic memory.
// If storer is non-nil, lines starting with EMERGING_SKILL: are parsed and stored as procedures.
func NewSemanticizationFunc(c *Client, storer EmergedSkillStorer) model.SemanticizationFunc {
	return func(ctx context.Context, memories []model.Memory) (*model.Memory, error) {
		if len(memories) == 0 {
			return nil, nil
		}

		var sb strings.Builder
		sb.WriteString("/no_think\nConsolidate these episodic memories into a single reusable fact:\n\n")
		for i, m := range memories {
			project := m.Metadata["project"]
			prefix := ""
			if project != "" {
				prefix = fmt.Sprintf("[project=%s] ", project)
			}
			tags := ""
			if len(m.Tags) > 0 {
				tags = " [" + strings.Join(m.Tags, ", ") + "]"
			}
			fmt.Fprintf(&sb, "%d. %s%s%s\n", i+1, prefix, m.Content, tags)
		}

		reply, err := c.Chat(ctx, []Message{
			{Role: "system", Content: consolidateSystemPrompt},
			{Role: "user", Content: sb.String()},
		}, 0.3)
		if err != nil {
			return nil, fmt.Errorf("semanticize: %w", err)
		}

		reply = strings.TrimSpace(reply)
		if reply == "" || strings.EqualFold(reply, "discard") {
			return nil, nil
		}

		// Parse and store emerging skills, then remove those lines from the reply.
		reply = parseAndStoreEmergedSkills(ctx, reply, storer)

		if reply == "" || strings.EqualFold(reply, "discard") {
			return nil, nil
		}

		// Detect the COMMON: prefix — the LLM flags cross-project insights.
		isCommon := false
		if strings.HasPrefix(strings.ToUpper(reply), strings.ToUpper(commonPrefix)) {
			isCommon = true
			reply = strings.TrimSpace(reply[len(commonPrefix):])
		}

		tagSet := make(map[string]bool)
		for _, m := range memories {
			for _, t := range m.Tags {
				tagSet[t] = true
			}
		}
		var tags []string
		for t := range tagSet {
			tags = append(tags, t)
		}

		meta := map[string]string{"source": "consolidation"}
		if isCommon {
			meta["project"] = "common"
		} else if p := commonProject(memories); p != "" {
			meta["project"] = p
		}

		return &model.Memory{
			Content:  reply,
			Salience: 0.7,
			Tags:     tags,
			Metadata: meta,
		}, nil
	}
}

// commonProject returns the shared project if every memory with a project
// field references the same one. Returns "" if mixed or absent.
func commonProject(memories []model.Memory) string {
	project := ""
	for _, m := range memories {
		p := m.Metadata["project"]
		if p == "" {
			continue
		}
		if project == "" {
			project = p
		} else if project != p {
			return ""
		}
	}
	return project
}
