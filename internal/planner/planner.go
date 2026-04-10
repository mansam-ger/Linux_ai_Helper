package planner

import (
	"strings"
	"eugen/internal/config"
	"eugen/internal/inference"
)

// Planner handles breaking down complex tasks into scripts.
type Planner struct {
	backend inference.Backend
	cfg     *config.EugenConfig
}

// NewPlanner creates a new planner instance.
func NewPlanner(backend inference.Backend, cfg *config.EugenConfig) *Planner {
	return &Planner{backend: backend, cfg: cfg}
}

// CreatePlan asks the LLM to generate a strict checklist of commands.
func (p *Planner) CreatePlan(systemContext, userTask string) ([]string, string, error) {
	planPrompt := p.cfg.RenderPrompt(p.cfg.PromptPlan, map[string]string{
		"context": systemContext,
	})

	resp, err := p.backend.Generate(planPrompt, userTask, nil)
	if err != nil {
		return nil, "", err
	}

	lines := strings.Split(resp, "\n")
	var commands []string
	
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "CMD: ") {
			cmdStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "CMD: "))
			cmdStr = strings.Trim(cmdStr, "`") // just in case the LLM used backticks anyway
			if cmdStr != "" {
				commands = append(commands, cmdStr)
			}
		}
	}

	return commands, resp, nil
}
