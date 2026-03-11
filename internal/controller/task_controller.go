/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentsv1 "github.com/ezequiel/agent-platform/api/v1"
	"github.com/ezequiel/agent-platform/scheduler"
)

// TaskReconciler reconciles a Task object.
type TaskReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Scheduler *scheduler.Scheduler
}

// +kubebuilder:rbac:groups=agents.platform,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.platform,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.platform,resources=tasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=agents.platform,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=agents.platform,resources=budgets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=agents.platform,resources=budgets/status,verbs=get;update;patch

// Reconcile handles Task CR events.
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var task agentsv1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Skip tasks that are already in a terminal or scheduled state.
	switch task.Status.Phase {
	case "scheduled", "completed", "failed":
		return ctrl.Result{}, nil
	}

	// Set phase to pending if empty.
	if task.Status.Phase == "" {
		task.Status.Phase = "pending"
	}

	// Attempt to schedule the task.
	result, err := r.Scheduler.Schedule(task)
	if err != nil {
		task.Status.Phase = "failed"
		task.Status.Reason = err.Error()
		log.Info("task scheduling failed", "task", task.Name, "reason", err.Error())

		if updateErr := r.Status().Update(ctx, &task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Scheduling succeeded.
	task.Status.Phase = "scheduled"
	task.Status.AssignedAgent = result.Agent.Name
	task.Status.Reason = result.Reason

	if result.Fallback {
		log.Info("agent budget exceeded, falling back",
			"task", task.Name,
			"from", task.Spec.Team,
			"to", result.Agent.Name,
		)
	}

	log.Info("task scheduled",
		"task", task.Name,
		"agent", result.Agent.Name,
		"fallback", result.Fallback,
		"reason", result.Reason,
	)

	// Update budget used by the task cost.
	if result.Agent.Spec.BudgetRef != "" {
		var budget agentsv1.Budget
		budgetKey := types.NamespacedName{
			Name:      result.Agent.Spec.BudgetRef,
			Namespace: task.Namespace,
		}
		if err := r.Get(ctx, budgetKey, &budget); err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		} else {
			budget.Status.Used += task.Spec.Cost
			budget.Status.Remaining = max(budget.Spec.Limit-budget.Status.Used, 0)
			if err := r.Status().Update(ctx, &budget); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if err := r.Status().Update(ctx, &task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentsv1.Task{}).
		Named("task").
		Complete(r)
}
