package middleware

import (
	"context"
	"regexp"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

// ContentClassifier is a middleware that classifies message content by
// sensitivity level and task type, optionally rerouting to a different model.
type ContentClassifier struct {
	cfg              config.ContentClassificationConfig
	sensitivityRules []sensitivityRule
}

type sensitivityRule struct {
	name     string
	patterns []*regexp.Regexp
	keywords []string
	routeTo  string
}

// NewContentClassifier builds a classifier from config.
func NewContentClassifier(cfg config.ContentClassificationConfig) *ContentClassifier {
	cc := &ContentClassifier{cfg: cfg}
	for name, level := range cfg.SensitivityLevels {
		rule := sensitivityRule{
			name:     name,
			keywords: level.Keywords,
			routeTo:  level.RouteTo,
		}
		for _, p := range level.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			rule.patterns = append(rule.patterns, re)
		}
		cc.sensitivityRules = append(cc.sensitivityRules, rule)
	}
	return cc
}

func (c *ContentClassifier) Name() string { return "content-classifier" }

func (c *ContentClassifier) ProcessRequest(_ context.Context, req *provider.ChatRequest, meta *RequestMeta) error {
	if !c.cfg.Enabled {
		return nil
	}

	// Concatenate user messages for scanning.
	var text strings.Builder
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			text.WriteString(msg.Content)
			text.WriteString(" ")
		}
	}
	content := text.String()

	// Check sensitivity rules.
	for _, rule := range c.sensitivityRules {
		if matchesSensitivity(content, rule) {
			meta.Tags["sensitivity"] = rule.name
			if rule.routeTo != "" {
				provID, model := provider.ParseModelString(rule.routeTo)
				if provID != "" {
					meta.ProviderID = provID
					meta.ModelName = model
				}
			}
			break // first match wins
		}
	}

	// Check task-type routing.
	if len(c.cfg.TaskTypeRoutes) > 0 {
		taskType := classifyTaskType(content)
		if taskType != "" {
			meta.Tags["task"] = taskType
			if routeTo, ok := c.cfg.TaskTypeRoutes[taskType]; ok {
				provID, model := provider.ParseModelString(routeTo)
				if provID != "" {
					meta.ProviderID = provID
					meta.ModelName = model
				}
			}
		}
	}

	return nil
}

func (c *ContentClassifier) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	return nil
}

func matchesSensitivity(text string, rule sensitivityRule) bool {
	for _, re := range rule.patterns {
		if re.MatchString(text) {
			return true
		}
	}
	lower := strings.ToLower(text)
	for _, kw := range rule.keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// classifyTaskType does basic keyword-based task classification.
// This mirrors the categories from agent.AssessTask() but is independent.
func classifyTaskType(text string) string {
	lower := strings.ToLower(text)

	securityKeywords := []string{"vulnerability", "cve", "exploit", "security", "pentest", "xss", "sql injection", "csrf"}
	for _, kw := range securityKeywords {
		if strings.Contains(lower, kw) {
			return "security"
		}
	}

	codingKeywords := []string{"write code", "implement", "refactor", "debug", "function", "class", "method", "api endpoint"}
	for _, kw := range codingKeywords {
		if strings.Contains(lower, kw) {
			return "coding"
		}
	}

	toolKeywords := []string{"run command", "execute", "shell", "bash", "terminal", "git ", "docker"}
	for _, kw := range toolKeywords {
		if strings.Contains(lower, kw) {
			return "tool-heavy"
		}
	}

	creativeKeywords := []string{"write a story", "poem", "creative", "brainstorm", "imagine"}
	for _, kw := range creativeKeywords {
		if strings.Contains(lower, kw) {
			return "creative"
		}
	}

	return ""
}
