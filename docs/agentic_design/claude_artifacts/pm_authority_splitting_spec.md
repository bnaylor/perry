# PM Authority Splitting Specification v1.0

## The Sovereign Loop Platform — Governance Layer

---

## 1. The Problem: The PM Has God Mode

In the current architecture, the PM Agent is responsible for:

- Translating user vision into requirements (PRD/CUJ authoring)
- Filtering communication between the user and the technical team
- Resolving conflicts between agents
- Maintaining the Approved Library List
- Routing tasks to the correct compute tier (local vs. cloud)
- Scoring task complexity for dispatch decisions
- Approving non-standard dependencies
- Deciding when to escalate to the user
- Managing the overall state of the project

This is too much authority in one agent for two reasons:

**Security risk**: If the PM hallucinates a policy decision, misinterprets user intent, or is manipulated by cleverly crafted input from another agent, everything downstream follows obediently. There's no check on the PM's authority except the user, who the PM is explicitly designed to shield from low-level details.

**Cognitive overload**: Even frontier models degrade when asked to hold too many roles simultaneously. A PM agent that's simultaneously writing requirements, making dispatch decisions, and resolving dependency conflicts will do all three worse than focused agents would do each one individually.

The solution is to decompose the PM into **three distinct components** with clearly separated authorities, where some of those components don't even need to be LLM-powered.

---

## 2. The Decomposition

```
                         ┌──────────┐
                         │   USER   │
                         └────┬─────┘
                              │
                    ┌─────────▼──────────┐
                    │                    │
                    │   THE STRATEGIST   │  ← LLM-powered (Tier 1)
                    │   (Product Mind)   │     Owns intent, requirements,
                    │                    │     user communication
                    └─────────┬──────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
    ┌─────────▼────────┐     │    ┌──────────▼──────────┐
    │                  │     │    │                     │
    │  THE DISPATCHER  │     │    │   THE POLICY        │
    │  (Task Router)   │     │    │   ENGINE            │
    │                  │     │    │   (Rule Enforcer)   │
    │  Deterministic/  │     │    │                     │
    │  Light LLM       │     │    │   Deterministic     │
    └─────────┬────────┘     │    └──────────┬──────────┘
              │               │               │
              │       ┌───────▼───────┐       │
              │       │ Technical     │       │
              └──────►│ Agents        │◄──────┘
                      │ (Researcher,  │
                      │  Coder, etc.) │
                      └───────────────┘
```

### The Three Components

| Component | Authority | LLM Required? | Trust Level |
|-----------|-----------|---------------|-------------|
| **The Strategist** | Intent, requirements, user communication, conflict resolution | Yes — Tier 1 Cloud | High (represents user intent) |
| **The Dispatcher** | Task routing, compute tier selection, retry management | Minimal — rules + light classifier | Medium (operational decisions, not strategic) |
| **The Policy Engine** | Dependency approval, security constraints, budget enforcement | No — purely deterministic | Highest (cannot be overridden by LLM judgment) |

---

## 3. The Strategist (Product Mind)

### What It Owns

The Strategist is the only component that talks to the user and the only component that produces requirements documents. It is the "soul" of the project — it understands *why* the user wants something and translates that into *what* needs to be built.

### Responsibilities

- **Requirement Synthesis**: Translates user vision into PRDs and CUJs.
- **User Communication**: Concise, decision-oriented updates. Shields the user from implementation noise.
- **Conflict Resolution**: When agents disagree (Researcher finds conflicting info, Coder hits a dead end), the Strategist makes the strategic call.
- **Scope Decisions**: Determines what's in scope vs. out of scope for a given task.
- **Open Question Resolution**: Answers the `open_questions` from Context Packets and Auditor escalations.

### What It Does NOT Own

- **It does not route tasks.** It doesn't decide whether a task runs locally or in the cloud. It doesn't assess complexity scores.
- **It does not approve dependencies.** It doesn't maintain the Approved Library List or decide whether `pandas` is safe.
- **It does not enforce budgets.** It doesn't track token spend or decide when to pause.
- **It does not manage retry logic.** It doesn't count how many times the Coder has failed the Auditor.

### Communication Protocol

```
TO THE USER:
  - Status updates: "The team has a working draft. One open question: 
    should the output include timestamps? [Yes/No]"
  - Escalations: "The Auditor flagged a security concern with the approach.
    Here's the tradeoff: [option A] is faster but requires broader network
    access. [option B] is safer but adds complexity. Your call."
  - Completion: "Task complete. Here's what was built: [summary]. 
    The Auditor approved it with risk level LOW."

TO THE RESEARCHER:
  - Task briefs: "I need you to research the OpenWeatherMap API. 
    Specifically: authentication method, rate limits, and the current 
    weather endpoint. The output format is defined in the Context Packet
    schema."

TO THE CODER (via Context Packet):
  - Requirements only flow through the Context Packet's task_reference
    section. The Strategist does not communicate directly with the Coder.

TO THE DISPATCHER:
  - Task definitions: "Here's a new task [TASK-0042] with these
    requirements. It's ready for dispatch."
  - Scope changes: "TASK-0042 now also needs to handle multiple cities.
    Update the Context Packet accordingly."
```

### System Prompt (Strategist Agent)

```
Role: You are the Strategist Agent — the product mind of a multi-agent 
development platform. You represent the user's intent and translate their 
vision into actionable requirements.

## Your Authority
- You OWN requirements: PRDs, CUJs, and task definitions.
- You OWN user communication: all messages to the user come through you.
- You OWN conflict resolution: when agents disagree, you make the call.
- You OWN scope: you decide what's in and out of scope.

## Your Boundaries
- You DO NOT route tasks to compute tiers. The Dispatcher handles this.
- You DO NOT approve or reject libraries. The Policy Engine handles this.
- You DO NOT track budgets or token spend. The Policy Engine handles this.
- You DO NOT manage retry counts or failure recovery. The Dispatcher 
  handles this.

## Decision Framework
When making decisions, prioritize in this order:
1. User Safety (never compromise the "house keys")
2. Correctness (it must actually work)
3. Simplicity (simpler is almost always better)
4. Speed (only after the above are satisfied)

## Communication Rules
- To the User: Concise, decision-oriented. Present options, not problems.
  Always frame escalations as choices with clear tradeoffs.
- To the Researcher: Specific task briefs. State exactly what information
  is needed and in what format.
- To the Dispatcher: Structured task definitions. Include task ID, 
  requirements summary, and priority level.
- You NEVER communicate directly with the Coder. Requirements flow 
  through the Context Packet only.

## Escalation from Other Components
When the Dispatcher or Policy Engine escalates to you:
- Dispatcher escalation: A task has failed too many times. You must decide
  whether to redefine the task, relax constraints (with user approval), 
  or abort.
- Policy Engine escalation: A dependency or operation was requested that's
  outside policy. You evaluate whether to request a user override.
- Auditor escalation: The Auditor has questions that require strategic
  judgment. You answer them or escalate to the user.
```

---

## 4. The Dispatcher (Task Router)

### What It Owns

The Dispatcher is an operational traffic controller. It decides *where* and *when* tasks run, manages retry logic, and tracks the execution lifecycle. It is deliberately simple — most of its logic is rule-based, with an optional lightweight LLM for complexity scoring.

### Responsibilities

- **Compute Tier Routing**: Assigns tasks to Tier 1 (Cloud) or Tier 2 (Local) based on complexity.
- **Retry Management**: Tracks failure counts and enforces circuit breakers.
- **Queue Management**: Orders tasks by priority and dependency.
- **Escalation Triggering**: When retry limits are breached, escalates to the Strategist.
- **Performance Tracking**: Records execution times, failure rates, and cost per task.

### What It Does NOT Own

- **It does not define requirements.** It doesn't know *why* a task exists.
- **It does not communicate with the user.** It never talks to the user directly.
- **It does not make policy decisions.** It doesn't decide whether a library is safe.
- **It does not resolve conflicts.** If two agents disagree, it routes the conflict to the Strategist.

### Implementation: Mostly Deterministic

The Dispatcher should be **as little LLM as possible**. The core routing logic is a decision tree, not a reasoning task.

```python
from dataclasses import dataclass, field
from enum import Enum
from datetime import datetime, timezone
from typing import Optional


class ComputeTier(Enum):
    LOCAL = "tier_2_local"
    CLOUD = "tier_1_cloud"


class TaskStatus(Enum):
    PENDING = "pending"
    DISPATCHED = "dispatched"
    IN_PROGRESS = "in_progress"
    AUDITING = "auditing"
    COMPLETED = "completed"
    FAILED = "failed"
    ESCALATED = "escalated"


class TaskCategory(Enum):
    """
    Task categories that map to complexity heuristics.
    The Strategist assigns these when creating tasks.
    """
    BOILERPLATE = "boilerplate"        # Template code, standard patterns
    UNIT_TESTS = "unit_tests"          # Test generation
    REFACTORING = "refactoring"        # Restructuring existing code
    API_INTEGRATION = "api_integration"  # Connecting to external services
    DATA_TRANSFORMATION = "data_transformation"  # ETL, format conversion
    SYSTEM_DESIGN = "system_design"    # Architecture, novel solutions
    SECURITY_SENSITIVE = "security_sensitive"  # Auth, crypto, access control
    DOCUMENTATION = "documentation"    # Docs, READMEs, comments


# ── ROUTING RULES ──────────────────────────────────────────────────

# Categories that ALWAYS run on Tier 1 (Cloud) — no exceptions
CLOUD_ONLY_CATEGORIES = {
    TaskCategory.SYSTEM_DESIGN,
    TaskCategory.SECURITY_SENSITIVE,
}

# Categories that START on Tier 2 (Local) and escalate on failure
LOCAL_FIRST_CATEGORIES = {
    TaskCategory.BOILERPLATE,
    TaskCategory.UNIT_TESTS,
    TaskCategory.REFACTORING,
    TaskCategory.DATA_TRANSFORMATION,
    TaskCategory.DOCUMENTATION,
}

# Categories that route based on estimated complexity
COMPLEXITY_ROUTED_CATEGORIES = {
    TaskCategory.API_INTEGRATION,
}


@dataclass
class TaskRecord:
    task_id: str
    category: TaskCategory
    priority: int  # 1 (highest) to 5 (lowest)
    strategist_summary: str
    assigned_tier: Optional[ComputeTier] = None
    status: TaskStatus = TaskStatus.PENDING
    attempt_count: int = 0
    failure_history: list[dict] = field(default_factory=list)
    created_at: str = field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat()
    )
    completed_at: Optional[str] = None
    estimated_tokens: Optional[int] = None
    actual_tokens: Optional[int] = None
    actual_cost_usd: Optional[float] = None


@dataclass 
class DispatcherConfig:
    """Loaded from config file, not hardcoded."""
    max_deterministic_retries: int = 3
    max_llm_retries: int = 2
    max_total_attempts: int = 5
    local_model_token_limit: int = 8000  # Max output tokens for local models
    complexity_threshold: float = 0.7     # Above this → Cloud
    budget_alert_threshold_usd: float = 5.0  # Per-task alert
    budget_hard_limit_usd: float = 20.0      # Per-task hard stop


class Dispatcher:
    def __init__(self, config: DispatcherConfig):
        self.config = config
        self.task_queue: list[TaskRecord] = []
        self.active_tasks: dict[str, TaskRecord] = {}

    def route_task(self, task: TaskRecord) -> ComputeTier:
        """
        Determine compute tier for a task.
        Pure logic — no LLM call needed for most cases.
        """
        # Rule 1: Cloud-only categories are non-negotiable
        if task.category in CLOUD_ONLY_CATEGORIES:
            return ComputeTier.CLOUD

        # Rule 2: Previously failed on Local → promote to Cloud
        local_failures = sum(
            1 for f in task.failure_history
            if f.get("tier") == ComputeTier.LOCAL.value
        )
        if local_failures >= 2:
            return ComputeTier.CLOUD

        # Rule 3: Local-first categories start local
        if task.category in LOCAL_FIRST_CATEGORIES:
            return ComputeTier.LOCAL

        # Rule 4: Complexity-routed categories use heuristics
        if task.category in COMPLEXITY_ROUTED_CATEGORIES:
            score = self._estimate_complexity(task)
            if score >= self.config.complexity_threshold:
                return ComputeTier.CLOUD
            return ComputeTier.LOCAL

        # Default: Cloud (fail-safe — when in doubt, use the smarter model)
        return ComputeTier.CLOUD

    def _estimate_complexity(self, task: TaskRecord) -> float:
        """
        Rule-based complexity estimation. Returns 0.0 - 1.0.
        
        This is deliberately simple. If you find yourself wanting to make
        this an LLM call, consider whether the category assignments are
        granular enough instead.
        """
        score = 0.5  # baseline

        summary_lower = task.strategist_summary.lower()

        # Heuristic signals that increase complexity
        complexity_signals = [
            ("oauth", 0.2),
            ("authentication", 0.15),
            ("pagination", 0.1),
            ("rate limit", 0.1),
            ("retry", 0.1),
            ("webhook", 0.2),
            ("streaming", 0.15),
            ("websocket", 0.2),
            ("graphql", 0.15),
            ("multi-step", 0.15),
            ("error handling", 0.05),
            ("concurrent", 0.15),
            ("async", 0.1),
        ]

        for signal, weight in complexity_signals:
            if signal in summary_lower:
                score += weight

        # Heuristic signals that decrease complexity
        simplicity_signals = [
            ("simple", -0.15),
            ("basic", -0.15),
            ("single endpoint", -0.1),
            ("GET only", -0.1),
            ("read-only", -0.1),
        ]

        for signal, weight in simplicity_signals:
            if signal in summary_lower:
                score += weight

        return max(0.0, min(1.0, score))

    def record_attempt_result(
        self, task_id: str, success: bool, tier: ComputeTier,
        failure_reason: str = "", tokens_used: int = 0,
        cost_usd: float = 0.0
    ) -> dict:
        """
        Record the result of a task attempt. Returns an action directive.
        
        Possible actions:
        - {"action": "retry", "tier": ComputeTier}
        - {"action": "escalate_to_strategist", "reason": str}
        - {"action": "complete"}
        - {"action": "budget_exceeded", "spent": float, "limit": float}
        """
        task = self.active_tasks.get(task_id)
        if not task:
            return {"action": "error", "reason": f"Unknown task: {task_id}"}

        task.attempt_count += 1
        task.actual_tokens = (task.actual_tokens or 0) + tokens_used
        task.actual_cost_usd = (task.actual_cost_usd or 0.0) + cost_usd

        if success:
            task.status = TaskStatus.COMPLETED
            task.completed_at = datetime.now(timezone.utc).isoformat()
            return {"action": "complete"}

        # Record failure
        task.failure_history.append({
            "attempt": task.attempt_count,
            "tier": tier.value,
            "reason": failure_reason,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        })

        # Budget check
        if task.actual_cost_usd >= self.config.budget_hard_limit_usd:
            task.status = TaskStatus.ESCALATED
            return {
                "action": "budget_exceeded",
                "spent": task.actual_cost_usd,
                "limit": self.config.budget_hard_limit_usd,
            }

        # Retry limit check
        if task.attempt_count >= self.config.max_total_attempts:
            task.status = TaskStatus.ESCALATED
            return {
                "action": "escalate_to_strategist",
                "reason": f"Task exceeded maximum attempts "
                          f"({self.config.max_total_attempts}). "
                          f"Failure history: {task.failure_history}",
            }

        # Determine next action: retry (possibly on different tier) or escalate
        new_tier = self.route_task(task)  # Re-route considering failure history
        task.status = TaskStatus.PENDING
        return {"action": "retry", "tier": new_tier}

    def get_queue_status(self) -> dict:
        """Operational dashboard data."""
        return {
            "pending": len([t for t in self.active_tasks.values()
                           if t.status == TaskStatus.PENDING]),
            "in_progress": len([t for t in self.active_tasks.values()
                               if t.status == TaskStatus.IN_PROGRESS]),
            "completed": len([t for t in self.active_tasks.values()
                             if t.status == TaskStatus.COMPLETED]),
            "escalated": len([t for t in self.active_tasks.values()
                             if t.status == TaskStatus.ESCALATED]),
            "total_cost_usd": sum(
                t.actual_cost_usd or 0.0
                for t in self.active_tasks.values()
            ),
        }
```

### Why Not an LLM?

The Dispatcher's decisions are operational, not strategic. "Should this run locally or in the cloud?" is a question with clear, enumerable inputs (task category, failure history, budget) and a small number of outputs (Tier 1 or Tier 2). An LLM adds latency, cost, and an attack surface for no meaningful gain.

The one exception is the complexity scoring heuristic. If the keyword-based approach proves too crude, you could replace `_estimate_complexity()` with a lightweight local LLM call (Tier 2). But start with rules and only add intelligence where the rules demonstrably fail.

---

## 5. The Policy Engine (Rule Enforcer)

### What It Owns

The Policy Engine is the platform's immune system. It enforces hard rules that **no LLM is permitted to override**, regardless of context or reasoning. It is entirely deterministic — a configuration file plus enforcement code.

### Responsibilities

- **Dependency Allowlist Management**: Maintains the list of approved libraries. Auto-approves standard packages, blocks known-dangerous ones, and routes everything else to the Strategist for user approval.
- **Operation Policy Enforcement**: Defines the baseline set of permitted and prohibited operations that apply to ALL tasks, before task-specific constraints are added.
- **Budget Enforcement**: Tracks cumulative token spend and cost. Enforces hard limits.
- **Security Baselines**: Rules that cannot be relaxed without explicit user override (and some that can't be relaxed at all).
- **Rate Limiting**: Prevents runaway loops from consuming unbounded resources.

### What It Does NOT Own

- **It does not understand intent.** It doesn't know *why* a library is needed.
- **It does not communicate with the user.** Escalations go to the Strategist, who decides how to present them.
- **It does not make judgment calls.** Every decision is traceable to a config rule.

### Implementation: Configuration-Driven

```python
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional
from enum import Enum


class ApprovalStatus(Enum):
    APPROVED = "approved"          # On the allowlist, auto-approved
    REQUIRES_APPROVAL = "requires_approval"  # Not on list, needs Strategist/user
    BLOCKED = "blocked"            # Explicitly prohibited, no override


@dataclass
class PolicyConfig:
    """
    Loaded from a JSON/YAML config file that the USER controls.
    This is the platform's constitution — it defines what agents CAN and
    CANNOT do, regardless of what any LLM thinks is reasonable.
    """

    # ── DEPENDENCY POLICY ──────────────────────────────────────────

    approved_python_packages: set[str] = field(default_factory=lambda: {
        # HTTP & networking
        "requests", "httpx", "aiohttp", "urllib3",
        # Data handling
        "pydantic", "jsonschema", "pyyaml", "toml",
        "pandas", "numpy", "csv",
        # CLI
        "click", "typer", "argparse",
        # Testing
        "pytest", "unittest", "coverage",
        # Utilities
        "python-dotenv", "pathlib", "dataclasses",
        "tqdm", "rich",
        # File formats
        "openpyxl", "xlsxwriter", "pillow",
        "jinja2", "markdown",
    })

    blocked_python_packages: set[str] = field(default_factory=lambda: {
        # Code execution / eval
        "pyautogui", "pynput",  # GUI automation (exfiltration risk)
        "ctypes",               # Direct memory access
        "cffi",                 # Foreign function interface
        # Network tools that bypass normal HTTP
        "scapy",                # Raw packet manipulation
        "paramiko",             # SSH (unless explicitly needed)
        "fabric",               # Remote execution
        # Obfuscation
        "pyarmor", "cython",    # Code obfuscation
        "marshal",              # Binary serialization (code hiding)
    })

    approved_npm_packages: set[str] = field(default_factory=set)
    blocked_npm_packages: set[str] = field(default_factory=set)

    # ── OPERATION POLICY ───────────────────────────────────────────

    # Operations that are ALWAYS prohibited, regardless of task
    globally_prohibited_operations: set[str] = field(default_factory=lambda: {
        "network_listen",          # Never open a listening socket
        "modify_system_config",    # Never touch /etc or system files
        "install_global_package",  # Never install globally
        "access_other_workrooms",  # Never touch other tasks' data
        "raw_socket",              # Never open raw sockets
        "kernel_module",           # Never load kernel modules
        "privilege_escalation",    # Never sudo/su
        "cron_modification",       # Never modify scheduled tasks
    })

    # Operations that require explicit user override even if the
    # Strategist approves
    user_override_required: set[str] = field(default_factory=lambda: {
        "sql_write",           # Writing to databases
        "http_delete",         # Destructive API calls
        "spawn_subprocess",    # Running arbitrary commands
    })

    # ── BUDGET POLICY ──────────────────────────────────────────────

    per_task_budget_warn_usd: float = 2.0
    per_task_budget_hard_limit_usd: float = 10.0
    session_budget_warn_usd: float = 20.0
    session_budget_hard_limit_usd: float = 50.0
    max_tokens_per_llm_call: int = 16000

    # ── RATE LIMITING ──────────────────────────────────────────────

    max_auditor_runs_per_task: int = 5
    max_researcher_calls_per_task: int = 10
    max_coder_attempts_per_task: int = 5
    max_total_agent_calls_per_hour: int = 200

    # ── SECURITY BASELINES ─────────────────────────────────────────

    # These CANNOT be relaxed, even by the user
    immutable_rules: list[str] = field(default_factory=lambda: [
        "Executor containers must be ephemeral (no persistent state).",
        "Secrets must be injected via environment variables, never in code.",
        "The Coder must never have network access.",
        "The Auditor pipeline cannot be bypassed.",
        "All audit records must be persisted before execution.",
    ])

    # Docker security settings
    docker_no_privileged: bool = True
    docker_no_host_network: bool = True
    docker_read_only_rootfs: bool = True
    docker_memory_limit_mb: int = 512
    docker_cpu_limit: float = 1.0
    docker_timeout_seconds: int = 300

    @classmethod
    def load(cls, config_path: str) -> "PolicyConfig":
        """Load from a JSON config file."""
        path = Path(config_path)
        if path.exists():
            data = json.loads(path.read_text())
            config = cls()
            # Merge loaded config over defaults
            for key, value in data.items():
                if hasattr(config, key):
                    current = getattr(config, key)
                    if isinstance(current, set) and isinstance(value, list):
                        setattr(config, key, set(value))
                    else:
                        setattr(config, key, value)
            return config
        return cls()  # Defaults


class PolicyEngine:
    """
    Deterministic policy enforcement. No LLM. No judgment.
    Every decision maps to a config rule.
    """

    def __init__(self, config: PolicyConfig):
        self.config = config
        self._session_cost_usd: float = 0.0
        self._session_agent_calls: int = 0
        self._task_costs: dict[str, float] = {}

    # ── DEPENDENCY CHECKS ──────────────────────────────────────────

    def check_python_package(self, package_name: str) -> dict:
        """
        Check a Python package against policy.
        Returns: {"status": ApprovalStatus, "reason": str}
        """
        normalized = package_name.lower().strip()

        if normalized in self.config.blocked_python_packages:
            return {
                "status": ApprovalStatus.BLOCKED,
                "reason": f"Package '{normalized}' is on the blocked list. "
                          f"This cannot be overridden.",
                "override_possible": False,
            }

        if normalized in self.config.approved_python_packages:
            return {
                "status": ApprovalStatus.APPROVED,
                "reason": f"Package '{normalized}' is on the approved list.",
                "override_possible": False,  # N/A
            }

        return {
            "status": ApprovalStatus.REQUIRES_APPROVAL,
            "reason": f"Package '{normalized}' is not on the approved or "
                      f"blocked lists. Requires Strategist review and "
                      f"possible user approval.",
            "override_possible": True,
        }

    def check_dependency_list(self, packages: list[str]) -> dict:
        """Check a full requirements list. Returns summary."""
        results = {}
        needs_approval = []
        blocked = []

        for pkg in packages:
            result = self.check_python_package(pkg)
            results[pkg] = result
            if result["status"] == ApprovalStatus.REQUIRES_APPROVAL:
                needs_approval.append(pkg)
            elif result["status"] == ApprovalStatus.BLOCKED:
                blocked.append(pkg)

        return {
            "all_approved": len(needs_approval) == 0 and len(blocked) == 0,
            "needs_approval": needs_approval,
            "blocked": blocked,
            "details": results,
        }

    # ── OPERATION CHECKS ───────────────────────────────────────────

    def check_operation(self, operation: str) -> dict:
        """
        Check if an operation is allowed by global policy.
        Task-specific permissions are layered on top of this.
        """
        if operation in self.config.globally_prohibited_operations:
            return {
                "allowed": False,
                "reason": f"Operation '{operation}' is globally prohibited.",
                "override_possible": False,
            }

        if operation in self.config.user_override_required:
            return {
                "allowed": False,
                "reason": f"Operation '{operation}' requires explicit user "
                          f"override.",
                "override_possible": True,
                "escalate_to": "user",
            }

        return {
            "allowed": True,
            "reason": f"Operation '{operation}' is permitted by global policy.",
        }

    def validate_constraints(self, constraints: dict) -> dict:
        """
        Validate a Context Packet's constraints section against
        global policy. Returns violations.
        """
        violations = []

        permitted = set(constraints.get("permitted_operations", []))
        prohibited = set(constraints.get("prohibited_operations", []))

        # Check: no globally prohibited operations in the permitted list
        leaked = permitted & self.config.globally_prohibited_operations
        if leaked:
            violations.append({
                "type": "globally_prohibited_in_permitted",
                "detail": f"Operations {leaked} are globally prohibited "
                          f"but appear in permitted_operations.",
                "severity": "BLOCK",
            })

        # Check: user-override operations need confirmation
        needs_override = permitted & self.config.user_override_required
        if needs_override:
            violations.append({
                "type": "user_override_required",
                "detail": f"Operations {needs_override} require explicit "
                          f"user approval.",
                "severity": "ESCALATE",
            })

        # Check: globally prohibited should be in the prohibited list
        missing_prohibitions = (
            self.config.globally_prohibited_operations - prohibited
        )
        if missing_prohibitions:
            violations.append({
                "type": "missing_global_prohibitions",
                "detail": f"Global prohibitions {missing_prohibitions} "
                          f"not listed in prohibited_operations. "
                          f"They will be enforced regardless.",
                "severity": "WARN",
            })

        return {
            "valid": len([v for v in violations
                         if v["severity"] == "BLOCK"]) == 0,
            "violations": violations,
        }

    # ── BUDGET ENFORCEMENT ─────────────────────────────────────────

    def record_cost(self, task_id: str, cost_usd: float) -> dict:
        """
        Record a cost event and check against limits.
        Returns enforcement action.
        """
        self._session_cost_usd += cost_usd
        self._task_costs[task_id] = (
            self._task_costs.get(task_id, 0.0) + cost_usd
        )

        task_cost = self._task_costs[task_id]
        session_cost = self._session_cost_usd

        # Hard limits — immediate stop
        if task_cost >= self.config.per_task_budget_hard_limit_usd:
            return {
                "action": "HARD_STOP",
                "scope": "task",
                "reason": f"Task {task_id} has spent ${task_cost:.2f} "
                          f"(limit: ${self.config.per_task_budget_hard_limit_usd:.2f})",
                "task_cost": task_cost,
                "session_cost": session_cost,
            }

        if session_cost >= self.config.session_budget_hard_limit_usd:
            return {
                "action": "HARD_STOP",
                "scope": "session",
                "reason": f"Session has spent ${session_cost:.2f} "
                          f"(limit: ${self.config.session_budget_hard_limit_usd:.2f})",
                "task_cost": task_cost,
                "session_cost": session_cost,
            }

        # Warn thresholds — notify Strategist
        warnings = []
        if task_cost >= self.config.per_task_budget_warn_usd:
            warnings.append(
                f"Task {task_id} approaching limit: "
                f"${task_cost:.2f} / ${self.config.per_task_budget_hard_limit_usd:.2f}"
            )
        if session_cost >= self.config.session_budget_warn_usd:
            warnings.append(
                f"Session approaching limit: "
                f"${session_cost:.2f} / ${self.config.session_budget_hard_limit_usd:.2f}"
            )

        if warnings:
            return {
                "action": "WARN",
                "warnings": warnings,
                "task_cost": task_cost,
                "session_cost": session_cost,
            }

        return {
            "action": "OK",
            "task_cost": task_cost,
            "session_cost": session_cost,
        }

    # ── RATE LIMITING ──────────────────────────────────────────────

    def check_rate_limit(
        self, task_id: str, agent_type: str, current_count: int
    ) -> dict:
        """Check if an agent has exceeded its per-task call limit."""
        limits = {
            "auditor": self.config.max_auditor_runs_per_task,
            "researcher": self.config.max_researcher_calls_per_task,
            "coder": self.config.max_coder_attempts_per_task,
        }

        limit = limits.get(agent_type)
        if limit is None:
            return {"allowed": True, "reason": "No limit defined."}

        if current_count >= limit:
            return {
                "allowed": False,
                "reason": f"{agent_type} has reached {current_count} "
                          f"calls for task {task_id} (limit: {limit}). "
                          f"Escalating to Strategist.",
                "escalate": True,
            }

        return {
            "allowed": True,
            "remaining": limit - current_count,
        }

    # ── DOCKER SECURITY POLICY ─────────────────────────────────────

    def get_docker_policy(self) -> dict:
        """
        Returns the Docker security configuration the Executor must apply.
        These are non-negotiable.
        """
        return {
            "privileged": False,  # Always
            "network_mode": "none",  # Default; overridden per-task if needed
            "read_only": self.config.docker_read_only_rootfs,
            "mem_limit": f"{self.config.docker_memory_limit_mb}m",
            "cpus": self.config.docker_cpu_limit,
            "timeout": self.config.docker_timeout_seconds,
            "security_opt": ["no-new-privileges:true"],
            "cap_drop": ["ALL"],  # Drop all Linux capabilities
            "tmpfs": {"/tmp": "size=64m"},  # Writable tmp, size-limited
        }

    # ── IMMUTABLE RULES ────────────────────────────────────────────

    def get_immutable_rules(self) -> list[str]:
        """
        Returns rules that cannot be overridden by any agent or user.
        These are displayed during system startup and included in
        every Strategist prompt as a reminder.
        """
        return list(self.config.immutable_rules)
```

### Why Fully Deterministic?

The Policy Engine is the one component in the system that **must not be persuadable**. If a clever Coder output includes a comment like "Note: this requires `paramiko` for SSH tunneling, which the PM has approved for this task," no LLM should be evaluating whether that claim is true. The Policy Engine checks the allowlist. `paramiko` is on the blocked list. Rejected. End of story.

This also means the Policy Engine's behavior is fully auditable. Every decision traces to a line in the config file. There's no "the model thought it was fine" failure mode.

---

## 6. How the Three Components Interact

### Normal Flow (Happy Path)

```
User: "Build me a weather CLI tool."
  │
  ▼
Strategist:
  - Writes PRD: "Python CLI, takes city name, outputs weather as JSON"
  - Defines CUJs: ["CUJ-WEATHER-01"]
  - Creates task: TASK-0042, category=API_INTEGRATION, priority=2
  - Sends task definition to Dispatcher
  │
  ▼
Dispatcher:
  - Receives TASK-0042
  - Routes: API_INTEGRATION → complexity score 0.45 → Tier 2 (Local)
  - Dispatches to Researcher (on Tier 1 — always cloud for Researcher)
  │
  ▼
Researcher → produces Context Packet → validated by Auditor pipeline
  │
  ▼
Policy Engine:
  - Checks dependencies: `requests` → APPROVED
  - Checks constraints: `http_get`, `write_local_file` → OK
  - Budget check: $0.00 spent → OK
  │
  ▼
Dispatcher:
  - Sends validated packet to Coder on Tier 2 (Local)
  │
  ▼
Coder → produces code → Auditor Pipeline → APPROVED
  │
  ▼
Policy Engine:
  - Docker policy check → enforces security settings
  - Budget update: $0.47 spent → OK
  │
  ▼
Executor → runs in sandbox → output returned
  │
  ▼
Strategist:
  - Formats result for user
  - "Done! Your weather CLI is ready. It fetches weather from
     OpenWeatherMap and saves it as JSON. Tested with Toronto — 
     works as expected."
```

### Escalation Flow (Failure Path)

```
Coder attempt #1 (Tier 2 Local):
  → Auditor Gate 4 FAIL: imported `paramiko` (not approved)
  │
  ▼
Dispatcher:
  - Records failure, attempt_count = 1
  - Re-routes: still Tier 2 (first failure)
  - Returns structured error to Coder
  │
  ▼
Coder attempt #2 (Tier 2 Local):
  → Auditor Gate 5 FAIL: capability leap (spawn_subprocess)
  │
  ▼
Dispatcher:
  - Records failure, attempt_count = 2
  - Re-routes: local_failures = 2 → promote to Tier 1 (Cloud)
  │
  ▼
Coder attempt #3 (Tier 1 Cloud):
  → Auditor LLM Review: REVISE (excess functionality detected)
  │
  ▼
Dispatcher:
  - Records failure, attempt_count = 3
  - Still under max_total_attempts (5)
  - Returns revision items to Coder
  │
  ▼
Coder attempt #4 (Tier 1 Cloud):
  → Auditor: APPROVED
  │
  ▼
Dispatcher:
  - Records success
  - Total cost: $1.23 (under budget)
  - Sends to Executor
```

### Policy Override Flow

```
Researcher produces Context Packet:
  - Needs `paramiko` for SFTP upload (legitimate requirement)
  │
  ▼
Policy Engine:
  - check_python_package("paramiko") → BLOCKED
  - Returns: "Package 'paramiko' is on the blocked list."
  │
  ▼
Strategist:
  - Receives Policy Engine rejection
  - Evaluates: "This task genuinely requires SFTP. The user's 
    requirements can't be met without it."
  - Escalates to User: "The task needs SFTP access, which requires 
    a library (paramiko) that's on our restricted list because it 
    enables remote command execution. Options:
    (A) I can find an alternative approach (maybe HTTP upload instead?)
    (B) You can approve paramiko for this specific task. 
    What do you prefer?"
  │
  ▼
User: "Option B — approve paramiko for this task."
  │
  ▼
Strategist:
  - Records user override with timestamp and reason
  - Instructs Dispatcher to add paramiko to task-specific exceptions
  │
  ▼
Policy Engine:
  - Task-specific override recorded
  - paramiko approved for TASK-0042 ONLY
  - Global blocked list unchanged
```

---

## 7. Authority Matrix

This matrix shows exactly which component has authority over each decision, and what override path exists.

| Decision | Primary Authority | Can Override? | Override Path |
|----------|------------------|---------------|---------------|
| What to build (requirements) | Strategist | User only | User redefines vision |
| Which compute tier | Dispatcher | Strategist on escalation | Strategist can force cloud |
| Is this library safe? | Policy Engine | User only (via Strategist) | User approves exception |
| Is this operation allowed? | Policy Engine | User only (via Strategist) for some; NEVER for immutable rules | User approves exception |
| Has the budget been exceeded? | Policy Engine | User only | User increases budget |
| Does the code match intent? | Auditor (LLM) | Strategist can override with justification | Strategist accepts risk |
| Is the code secure? | Auditor (Deterministic) | NEVER | No override for deterministic failures |
| How to communicate with user? | Strategist | N/A | Strategist is sole user channel |
| When to retry vs. escalate? | Dispatcher | Strategist on escalation | Strategist redefines or aborts task |
| Docker security settings | Policy Engine | NEVER | Immutable |

### Key Principle: Authority Decreases with Proximity to Execution

```
User          ████████████████████████  (maximum authority)
Strategist    ████████████████████      (strategic authority)
Policy Engine ██████████████████        (enforcement authority — narrow but absolute)
Dispatcher    ██████████████            (operational authority)
Researcher    ████████████              (informational authority)
Coder         ████████                  (implementation authority)
Auditor       ██████████████████        (verification authority — can block, not approve)
Executor      ████                      (execution authority — minimal, sandboxed)
```

---

## 8. Migration Path from Monolithic PM

If you're building iteratively, you don't have to split all three components at once. Here's the recommended order:

### Phase 1: Extract the Policy Engine (Week 1)

This is the highest-value, lowest-risk extraction. The Policy Engine is purely deterministic — it's just a config file and enforcement code. You can build and test it independently.

**What changes**: The current PM stops making dependency approval decisions and budget checks. Those queries go to the Policy Engine instead.

**What doesn't change**: The PM still handles everything else.

### Phase 2: Extract the Dispatcher (Week 2-3)

Pull the routing logic, retry management, and lifecycle tracking out of the PM. The Dispatcher is mostly rule-based, so this is straightforward.

**What changes**: The PM stops tracking attempt counts, compute tier assignments, and failure recovery. It sends task definitions to the Dispatcher and receives escalations back.

**What doesn't change**: The PM still owns requirements, user communication, and conflict resolution.

### Phase 3: Rename the PM to Strategist (Week 3-4)

At this point, the PM has been slimmed down to its core role — product thinking. Rename it, update its system prompt to reflect the narrowed authority, and you're done.

**What changes**: The PM's system prompt shrinks significantly. It gains clear boundaries about what it does NOT own.

**What doesn't change**: The user experience. The Strategist still communicates the same way the PM always did.

---

## 9. AG2 Integration Notes

In AG2 terms, the three components map to:

| Component | AG2 Primitive | Notes |
|-----------|--------------|-------|
| Strategist | `AssistantAgent` | Full LLM-powered agent with system prompt. Tier 1 model. |
| Dispatcher | Custom Python class | NOT an AG2 agent. It's infrastructure code that the `GroupChatManager` calls. Inject it into the custom speaker selection function. |
| Policy Engine | Custom Python class | NOT an AG2 agent. It's a service that other agents query. Loaded at startup from config. No LLM. |

The key insight: **not everything needs to be an agent**. The Dispatcher and Policy Engine are better as plain services that the orchestration layer calls. Making them agents adds unnecessary complexity, token cost, and attack surface.
