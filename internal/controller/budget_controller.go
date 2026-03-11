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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentsv1 "github.com/ezequiel/agent-platform/api/v1"
	"github.com/ezequiel/agent-platform/scheduler"
)

// BudgetReconciler reconciles a Budget object.
type BudgetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Scheduler *scheduler.Scheduler
}

// +kubebuilder:rbac:groups=agents.platform,resources=budgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.platform,resources=budgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.platform,resources=budgets/finalizers,verbs=update

// Reconcile handles Budget CR events.
func (r *BudgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var budget agentsv1.Budget
	if err := r.Get(ctx, req.NamespacedName, &budget); err != nil {
		if errors.IsNotFound(err) {
			log.Info("budget deleted", "budget", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Sync budget to the scheduler.
	r.Scheduler.SyncBudget(budget)

	// Update remaining = limit - used.
	budget.Status.Remaining = max(budget.Spec.Limit-budget.Status.Used, 0)

	if err := r.Status().Update(ctx, &budget); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("budget status updated", "budget", budget.Name, "used", budget.Status.Used, "remaining", budget.Status.Remaining)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BudgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentsv1.Budget{}).
		Named("budget").
		Complete(r)
}
