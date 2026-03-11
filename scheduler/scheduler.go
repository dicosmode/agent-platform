package scheduler

import (
	"fmt"
	"slices"
	"sync"

	agentsv1 "github.com/ezequiel/agent-platform/api/v1"
)

// PoolPriority defines the fallback order for agent pools.
var PoolPriority = map[string]int{
	"team":   0,
	"shared": 1,
	"global": 2,
}

// Scheduler manages budget-aware task scheduling with pool-based fallback.
type Scheduler struct {
	mu           sync.RWMutex
	budgets      map[string]int
	budgetLimits map[string]int
	agents       map[string]agentsv1.Agent
}

// New creates a new Scheduler instance.
func New() *Scheduler {
	return &Scheduler{
		budgets:      make(map[string]int),
		budgetLimits: make(map[string]int),
		agents:       make(map[string]agentsv1.Agent),
	}
}

// ScheduleResult contains the result of a scheduling decision.
type ScheduleResult struct {
	Agent    agentsv1.Agent
	Fallback bool
	Reason   string
}

// RegisterAgent adds or updates an agent in the scheduler.
func (s *Scheduler) RegisterAgent(agent agentsv1.Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.Name] = agent
}

// UnregisterAgent removes an agent from the scheduler.
func (s *Scheduler) UnregisterAgent(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, name)
}

// SyncBudget updates the budget state from a Budget resource.
func (s *Scheduler) SyncBudget(budget agentsv1.Budget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := max(budget.Spec.Limit-budget.Status.Used, 0)
	s.budgets[budget.Name] = remaining
	s.budgetLimits[budget.Name] = budget.Spec.Limit
}

// GetBudgetUsed returns the used amount for a budget.
func (s *Scheduler) GetBudgetUsed(budgetName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit, hasLimit := s.budgetLimits[budgetName]
	remaining, hasRemaining := s.budgets[budgetName]
	if !hasLimit || !hasRemaining {
		return 0
	}
	return limit - remaining
}

// GetBudgetRemaining returns the remaining budget for a given budget name.
func (s *Scheduler) GetBudgetRemaining(budgetName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	remaining, ok := s.budgets[budgetName]
	if !ok {
		return 0
	}
	return remaining
}

// Schedule finds an appropriate agent for a task using budget-aware scheduling
// with pool-based fallback (team -> shared -> global).
func (s *Scheduler) Schedule(task agentsv1.Task) (*ScheduleResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := s.findAgentsBySkill(task.Spec.Skill)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no agents found with skill %q", task.Spec.Skill)
	}

	slices.SortFunc(candidates, func(a, b agentsv1.Agent) int {
		return poolScore(a) - poolScore(b)
	})

	var exhaustedAgents []string
	teamAgent := ""
	for _, agent := range candidates {
		if agent.Spec.Pool == "team" && teamAgent == "" {
			teamAgent = agent.Name
		}
		if s.hasBudget(agent.Spec.BudgetRef, task.Spec.Cost) {
			s.deductBudget(agent.Spec.BudgetRef, task.Spec.Cost)
			isFallback := teamAgent != "" && agent.Name != teamAgent
			reason := fmt.Sprintf("task assigned to %s", agent.Name)
			if isFallback {
				reason = fmt.Sprintf("%s budget exceeded, fallback to %s, task assigned", teamAgent, agent.Name)
			}
			return &ScheduleResult{
				Agent:    agent,
				Fallback: isFallback,
				Reason:   reason,
			}, nil
		}
		exhaustedAgents = append(exhaustedAgents, agent.Name)
	}

	return nil, fmt.Errorf("no budget available; exhausted agents: %v", exhaustedAgents)
}

func (s *Scheduler) findAgentsBySkill(skill string) []agentsv1.Agent {
	var result []agentsv1.Agent
	for _, agent := range s.agents {
		if slices.Contains(agent.Spec.Skills, skill) {
			result = append(result, agent)
		}
	}
	return result
}

func poolScore(agent agentsv1.Agent) int {
	base, ok := PoolPriority[agent.Spec.Pool]
	if !ok {
		return 99
	}
	return base
}

func (s *Scheduler) hasBudget(budgetRef string, cost int) bool {
	remaining, ok := s.budgets[budgetRef]
	if !ok {
		return false
	}
	return remaining >= cost
}

func (s *Scheduler) deductBudget(budgetRef string, cost int) {
	if remaining, ok := s.budgets[budgetRef]; ok {
		s.budgets[budgetRef] = remaining - cost
	}
}
