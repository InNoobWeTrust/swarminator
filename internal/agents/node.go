package agents

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

type NodeConfig struct {
	Name        string
	Description string
	Persona     string
	Role        string
	Intent      string
	Target      string
}

func NewNodeAgent(llm model.LLM, cfg NodeConfig) (agent.Agent, error) {
	instruction := fmt.Sprintf("You are a swarm node. Role=%s Intent=%s Target=%s.\n\nPersona:\n%s", cfg.Role, cfg.Intent, cfg.Target, cfg.Persona)
	return llmagent.New(llmagent.Config{
		Name:        cfg.Name,
		Model:       llm,
		Description: cfg.Description,
		Instruction: instruction,
	})
}

type Advisor interface {
	Advise(context.Context, string) (string, error)
}
