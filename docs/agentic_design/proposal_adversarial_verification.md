# Proposal: Adversarial Verification (The "Shadow Auditor")

## The Problem
Semantic auditing by LLMs often suffers from **approval bias**. When an agent's primary goal is to be "helpful," it tends to look for reasons why code *should* work rather than why it *shouldn't*. Traditional security scanners (AST, Bandit) catch known patterns but are blind to custom logic backdoors, subtle data exfiltration paths, or insecure state transitions specific to the application's domain.

## The Solution: The Shadow Auditor (Red Team Agent)
We propose a mandatory **Adversarial Verification** step in the Audit Pipeline. This introduces a dedicated agent—the **Shadow Auditor**—whose sole "win condition" is to break the Coder's implementation.

### 1. The Adversarial Persona & Incentives
Unlike the standard Auditor (which checks for intent and style), the Shadow Auditor is prompted as a **Malicious Adversary**.

- **Goal-Oriented Rejection:** The Shadow Auditor is told: *"Your success is measured by finding vulnerabilities. If this code is perfectly secure, you have failed. Find the flaw."*
- **Asymmetric Conflict:** This creates a natural "adversarial game" between the Coder (who wants to pass) and the Shadow Auditor (who wants to fail them). This conflict significantly reduces the chance of "hand-wavy" approvals.

### 2. The Exploit Report & Verification Loop
The Shadow Auditor does not just "reject" code with a comment; it must produce a structured **Exploit Report**:

- **Vulnerability Type:** (e.g., Logic Backdoor, Data Leak, Resource Exhaustion).
- **The "How":** A detailed explanation of the exploit path.
- **The Proof (PoC):** A specific set of inputs, environment variables, or a test script that demonstrates the flaw.
- **Verification:** The **Executor** runs the Shadow Auditor's PoC in the sandbox against the Coder's code. 
    - If the PoC succeeds (the exploit works), the task is routed back to `CODING` with the Exploit Report as a mandatory "fix" requirement.
    - If the PoC fails (the exploit doesn't work), the Shadow Auditor's finding is dismissed as a "False Positive," and the audit continues.

### 3. Asymmetric Layering in the FSM
The Shadow Auditor sits between the Deterministic Gates and the Semantic Auditor in the `AUDITING` state.

1. **Gate 1 (Deterministic):** AST, Bandit, Secrets scan (Local).
2. **Gate 2 (Adversarial):** **Shadow Auditor** attempts to break the code (Cloud LLM).
3. **Gate 3 (Semantic):** Standard Auditor verifies intent alignment and style (Cloud LLM).

By running the Shadow Auditor *before* the standard Auditor, we ensure that the "friendly" auditor is reviewing code that has already survived a targeted red-team attack.

## Impact on Security
- **Catches Domain-Specific Flaws:** Unlike a generic scanner, the Shadow Auditor understands the *logic* of the task and can spot vulnerabilities in custom business logic.
- **Forces Robustness:** The Coder agent quickly learns that "hacks" won't pass, leading to higher-quality, more defensive code over time.
- **Reduces Human Review Fatigue:** By the time a task reaches `HUMAN_REVIEW`, it has already been "battle-tested" by an adversarial agent, meaning the human only sees high-confidence, hardened code.

## Future Evolution (Phase 3+)
- **Multi-Model Red Teaming:** Using a different LLM provider for the Shadow Auditor than for the Coder/Auditor (e.g., Gemini vs. Claude) to prevent "shared model blind spots."
- **Automated Fuzzing:** The Shadow Auditor generates the *parameters* for an automated fuzzer that runs in the Executor sandbox.
