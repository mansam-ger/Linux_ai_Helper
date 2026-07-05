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

// CreateScript asks the LLM to generate a complete script with flow control.
func (p *Planner) CreateScript(systemContext, userTask string) (string, string, error) {
	scriptPrompt := p.cfg.RenderPrompt(p.cfg.PromptScript, map[string]string{
		"context": systemContext,
	})

	resp, err := p.backend.Generate(scriptPrompt, userTask, nil)
	if err != nil {
		return "", "", err
	}

	var scriptContent []string
	inBlock := false
	lines := strings.Split(resp, "\n")
	
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") {
			inBlock = !inBlock
			continue
		}
		if inBlock {
			scriptContent = append(scriptContent, l) // preserve original indentation
		}
	}

	// Fallback if no markdown block was found, but the LLM provided a raw script starting with a shebang
	if len(scriptContent) == 0 {
		foundShebang := false
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if !foundShebang {
				if strings.HasPrefix(trimmed, "#!") {
					foundShebang = true
					scriptContent = append(scriptContent, l)
				}
			} else {
				// Stop if it suddenly opens a markdown block (which shouldn't happen here)
				if strings.HasPrefix(trimmed, "```") {
					break
				}
				scriptContent = append(scriptContent, l)
			}
		}
	}

	// Fallback 2: The LLM hallucinated the old CMD: format
	if len(scriptContent) == 0 {
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "CMD: ") {
				cmdStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "CMD: "))
				cmdStr = strings.Trim(cmdStr, "`")
				if cmdStr != "" {
					scriptContent = append(scriptContent, cmdStr)
				}
			}
		}
	}

	return strings.TrimSpace(strings.Join(scriptContent, "\n")), resp, nil
}
