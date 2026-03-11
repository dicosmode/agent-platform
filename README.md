# Agent Platform — Budget-Aware Scheduling

Mock funcional de una plataforma de agentes sobre Kubernetes que simula scheduling con budget awareness y reasignación automática por fallback de pools.

## Arquitectura

```
                ┌───────────────┐
                │ Task Request  │
                └──────┬────────┘
                       │
                       ▼
                 TaskController
                       │
                       ▼
              Budget Scheduler
                       │
           ┌───────────┼───────────┐
           ▼           ▼           ▼
       Agent A     Agent B     Agent C
       Team Pool   Shared      Global
```

**Flujo:**
1. Se crea un `Task` CR con una skill requerida y un costo
2. El `TaskController` pide al `Scheduler` que asigne un agente
3. El `Scheduler` busca agentes con la skill, ordena por pool priority (team → shared → global)
4. Si el agente tiene budget → lo asigna y deduce el costo
5. Si no tiene budget → hace fallback al siguiente pool disponible

## Custom Resources

| CRD | Descripción |
|-----|-------------|
| **Agent** | Agente ejecutor con pool, skills y referencia a budget |
| **Skill** | Capacidad ejecutable con imagen de container y costo en tokens |
| **Budget** | Presupuesto con límite y tracking de uso |
| **Task** | Tarea solicitada con skill requerida, costo y team |

### Ejemplo de Agent
```yaml
apiVersion: agents.platform/v1
kind: Agent
metadata:
  name: agent-marketing
spec:
  pool: team
  skills: [summarize, translate]
  budgetRef: marketing-budget
  endpoint: http://agent-marketing:8080
```

### Ejemplo de Task
```yaml
apiVersion: agents.platform/v1
kind: Task
metadata:
  name: summarize-task
spec:
  skill: summarize
  cost: 10
  team: marketing
```

## Pools de Fallback

Tres niveles con orden de prioridad:

1. **team** — agentes del mismo equipo (mayor prioridad)
2. **shared** — agentes compartidos entre equipos
3. **global** — agentes globales (menor prioridad)

Cuando un agente no tiene budget suficiente, el scheduler busca automáticamente en el siguiente pool.

## Stack

- **Cluster:** kind (Kubernetes >= 1.29)
- **Runtime:** Go, controller-runtime, kubebuilder
- **Budget state:** in-memory (mock de Redis)
- **Tools:** kubectl, kustomize, make

## Estructura del Proyecto

```
agent-platform/
├── api/v1/                    # CRD type definitions
│   ├── agent_types.go
│   ├── skill_types.go
│   ├── budget_types.go
│   └── task_types.go
├── internal/controller/       # Kubernetes controllers
│   ├── agent_controller.go
│   ├── budget_controller.go
│   └── task_controller.go
├── scheduler/                 # Budget-aware scheduler
│   ├── scheduler.go
│   └── scheduler_test.go
├── manifests/                 # Sample CRs
│   ├── agents/
│   ├── budgets/
│   ├── skills/
│   └── tasks/
├── config/                    # Kubebuilder config (CRDs, RBAC, deploy)
├── cmd/main.go                # Entrypoint
├── kind-config.yaml           # Kind cluster config
└── Makefile
```

## Requisitos

- Go >= 1.21
- Docker
- kind
- kubectl
- kubebuilder
- kustomize
- make

## Quick Start

### 1. Crear cluster

```bash
make kind-create
```

### 2. Instalar CRDs

```bash
make install
```

### 3. Build y deploy del controller

```bash
make docker-build IMG=agent-platform:dev
kind load docker-image agent-platform:dev --name agent-platform
make deploy IMG=agent-platform:dev
```

### 4. Deploy de recursos de ejemplo

```bash
make deploy-samples
```

### 5. Crear una task y ver el resultado

```bash
make test-flow
```

Output esperado:

```
NAME             SKILL       PHASE       ASSIGNEDAGENT     COST
summarize-task   summarize   scheduled   agent-marketing   10
```

## Test de Fallback

Crear múltiples tasks para agotar el budget del team y observar el fallback:

```bash
# Crear 10 tasks de costo 10 (total = 100 = límite del marketing-budget)
for i in $(seq 1 10); do
  kubectl apply -f - <<EOF
apiVersion: agents.platform/v1
kind: Task
metadata:
  name: task-$i
spec:
  skill: summarize
  cost: 10
  team: marketing
EOF
done

# La task 11 debería hacer fallback a nlp-agent (shared pool)
kubectl apply -f - <<EOF
apiVersion: agents.platform/v1
kind: Task
metadata:
  name: task-overflow
spec:
  skill: summarize
  cost: 10
  team: marketing
EOF

# Verificar
kubectl get tasks task-overflow -o wide
# → ASSIGNEDAGENT: nlp-agent (fallback!)

kubectl get budgets -o wide
# → marketing-budget: USED=100, REMAINING=0
# → shared-budget:    USED=10,  REMAINING=490
```

Logs del controller:

```
agent budget exceeded, falling back  {"task":"task-overflow", "from":"marketing", "to":"nlp-agent"}
task scheduled  {"task":"task-overflow", "agent":"nlp-agent", "fallback":true}
```

## Unit Tests

```bash
go test ./scheduler/ -v
```

```
=== RUN   TestSchedule_HappyPath
=== RUN   TestSchedule_BudgetExhausted_Fallback
=== RUN   TestSchedule_NoBudgetAvailable
=== RUN   TestSchedule_NoAgentsWithSkill
=== RUN   TestSchedule_PoolOrder_TeamSharedGlobal
=== RUN   TestSchedule_PoolFallbackChain
=== RUN   TestSchedule_BudgetDeduction
PASS
```

## Comandos útiles

| Comando | Descripción |
|---------|-------------|
| `make kind-create` | Crear cluster kind |
| `make kind-delete` | Eliminar cluster kind |
| `make install` | Instalar CRDs |
| `make deploy IMG=agent-platform:dev` | Deploy del controller |
| `make deploy-samples` | Deploy de agents, budgets, skills |
| `make test-flow` | Crear task de prueba y ver resultado |
| `make docker-build IMG=agent-platform:dev` | Build de imagen Docker |
| `make kind-load` | Cargar imagen en kind |
| `kubectl get tasks -o wide` | Ver tasks con agent asignado |
| `kubectl get budgets -o wide` | Ver estado de budgets |
| `kubectl get agents -o wide` | Ver estado de agentes |
| `kubectl logs deploy/agent-platform-controller-manager -n agent-platform-system` | Ver logs |

## Cleanup

```bash
make undeploy
make kind-delete
```

## License

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

