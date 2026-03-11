package scheduler

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentsv1 "github.com/ezequiel/agent-platform/api/v1"
)

func newAgent(name, pool, budgetRef string, skills []string) agentsv1.Agent {
	return agentsv1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: agentsv1.AgentSpec{
			Pool:      pool,
			Skills:    skills,
			BudgetRef: budgetRef,
		},
	}
}

func newBudget(name string, limit, used int, pool string) agentsv1.Budget {
	return agentsv1.Budget{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: agentsv1.BudgetSpec{
			Limit: limit,
			Pool:  pool,
		},
		Status: agentsv1.BudgetStatus{
			Used: used,
		},
	}
}

func newTask(name string) agentsv1.Task {
	return agentsv1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: agentsv1.TaskSpec{
			Skill: "summarize",
			Cost:  10,
			Team:  "marketing",
		},
	}
}

func TestSchedule_HappyPath(t *testing.T) {
	s := New()

	agent := newAgent("agent-marketing", "team", "marketing-budget", []string{"summarize", "translate"})
	s.RegisterAgent(agent)

	budget := newBudget("marketing-budget", 100, 20, "team")
	s.SyncBudget(budget)

	task := newTask("task-1")
	result, err := s.Schedule(task)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Agent.Name != "agent-marketing" {
		t.Errorf("expected agent-marketing, got %s", result.Agent.Name)
	}
	if result.Fallback {
		t.Error("expected no fallback")
	}

	remaining := s.GetBudgetRemaining("marketing-budget")
	if remaining != 70 {
		t.Errorf("expected 70 remaining, got %d", remaining)
	}
}

func TestSchedule_BudgetExhausted_Fallback(t *testing.T) {
	s := New()

	teamAgent := newAgent("agent-marketing", "team", "marketing-budget", []string{"summarize"})
	s.RegisterAgent(teamAgent)

	sharedAgent := newAgent("nlp-agent", "shared", "shared-budget", []string{"summarize", "sentiment"})
	s.RegisterAgent(sharedAgent)

	s.SyncBudget(newBudget("marketing-budget", 100, 95, "team"))
	s.SyncBudget(newBudget("shared-budget", 500, 50, "shared"))

	task := newTask("task-1")
	result, err := s.Schedule(task)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Agent.Name != "nlp-agent" {
		t.Errorf("expected nlp-agent fallback, got %s", result.Agent.Name)
	}
	if !result.Fallback {
		t.Error("expected fallback=true")
	}
}

func TestSchedule_NoBudgetAvailable(t *testing.T) {
	s := New()

	agent := newAgent("agent-marketing", "team", "marketing-budget", []string{"summarize"})
	s.RegisterAgent(agent)

	s.SyncBudget(newBudget("marketing-budget", 100, 100, "team"))

	task := newTask("task-1")
	_, err := s.Schedule(task)
	if err == nil {
		t.Fatal("expected error for exhausted budget, got nil")
	}
}

func TestSchedule_NoAgentsWithSkill(t *testing.T) {
	s := New()

	agent := newAgent("agent-marketing", "team", "marketing-budget", []string{"translate"})
	s.RegisterAgent(agent)

	s.SyncBudget(newBudget("marketing-budget", 100, 0, "team"))

	task := newTask("task-1")
	_, err := s.Schedule(task)
	if err == nil {
		t.Fatal("expected error for no matching skill, got nil")
	}
}

func TestSchedule_PoolOrder_TeamSharedGlobal(t *testing.T) {
	s := New()

	globalAgent := newAgent("global-agent", "global", "global-budget", []string{"summarize"})
	sharedAgent := newAgent("shared-agent", "shared", "shared-budget", []string{"summarize"})
	teamAgent := newAgent("team-agent", "team", "team-budget", []string{"summarize"})

	s.RegisterAgent(globalAgent)
	s.RegisterAgent(sharedAgent)
	s.RegisterAgent(teamAgent)

	s.SyncBudget(newBudget("team-budget", 100, 0, "team"))
	s.SyncBudget(newBudget("shared-budget", 100, 0, "shared"))
	s.SyncBudget(newBudget("global-budget", 100, 0, "global"))

	task := newTask("task-1")
	result, err := s.Schedule(task)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Agent.Name != "team-agent" {
		t.Errorf("expected team-agent (highest priority pool), got %s", result.Agent.Name)
	}
}

func TestSchedule_PoolFallbackChain(t *testing.T) {
	s := New()

	teamAgent := newAgent("team-agent", "team", "team-budget", []string{"summarize"})
	sharedAgent := newAgent("shared-agent", "shared", "shared-budget", []string{"summarize"})
	globalAgent := newAgent("global-agent", "global", "global-budget", []string{"summarize"})

	s.RegisterAgent(teamAgent)
	s.RegisterAgent(sharedAgent)
	s.RegisterAgent(globalAgent)

	s.SyncBudget(newBudget("team-budget", 10, 10, "team"))
	s.SyncBudget(newBudget("shared-budget", 10, 10, "shared"))
	s.SyncBudget(newBudget("global-budget", 100, 0, "global"))

	task := newTask("task-1")
	result, err := s.Schedule(task)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Agent.Name != "global-agent" {
		t.Errorf("expected global-agent as last fallback, got %s", result.Agent.Name)
	}
	if !result.Fallback {
		t.Error("expected fallback=true")
	}
}

func TestSchedule_BudgetDeduction(t *testing.T) {
	s := New()

	agent := newAgent("agent-1", "team", "budget-1", []string{"summarize"})
	s.RegisterAgent(agent)
	s.SyncBudget(newBudget("budget-1", 100, 0, "team"))

	for i := range 10 {
		task := newTask("task")
		_, err := s.Schedule(task)
		if err != nil {
			t.Fatalf("iteration %d: expected no error, got: %v", i, err)
		}
	}

	remaining := s.GetBudgetRemaining("budget-1")
	if remaining != 0 {
		t.Errorf("expected 0 remaining after 10 tasks, got %d", remaining)
	}

	task := newTask("task-overflow")
	_, err := s.Schedule(task)
	if err == nil {
		t.Fatal("expected error after budget exhausted, got nil")
	}
}
