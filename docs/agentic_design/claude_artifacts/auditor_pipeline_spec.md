# Auditor Pipeline Specification v1.0

## The Sovereign Loop Platform — Verification Layer

---

## 1. The Problem with a Monolithic Auditor

The original architecture describes the Auditor as a single agent that performs:

- Capability Leap detection (did the code exceed its mandate?)
- Credential scanning (are secrets hardcoded?)
- Static analysis (security vulnerabilities via Bandit, etc.)
- Cross-reference auditing (does code match the Context Packet and PRD?)

This is too much responsibility for one LLM call. The failure modes are different, the tools are different, and — critically — **mixing deterministic checks with LLM judgment creates a single point of compromise**. If the Auditor LLM is manipulated (via injection in code comments, docstrings, or variable names), the deterministic checks go down with it.

The redesign splits the Auditor into a **pipeline** of discrete stages, each with its own tooling, pass/fail criteria, and failure-handling behavior.

---

## 2. Pipeline Architecture

```
Coder Output (code + requirements.txt + manifest)
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│                 DETERMINISTIC LAYER                      │
│            (No LLM. No judgment calls.                   │
│             Cannot be socially engineered.)               │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Gate 1:      │  │ Gate 2:      │  │ Gate 3:       │  │
│  │ Structural   │─►│ Secrets &    │─►│ Static        │  │
│  │ Conformance  │  │ Credential   │  │ Analysis      │  │
│  │              │  │ Scan         │  │               │  │
│  └──────────────┘  └──────────────┘  └───────────────┘  │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐                      │
│  │ Gate 4:      │  │ Gate 5:      │                      │
│  │ Dependency   │─►│ Capability   │                      │
│  │ Allowlist    │  │ Fence        │                      │
│  │              │  │ (AST-based)  │                      │
│  └──────────────┘  └──────────────┘                      │
└────────────────────────┬────────────────────────────────┘
                         │ ALL PASS
                         ▼
┌─────────────────────────────────────────────────────────┐
│                  LLM REASONING LAYER                     │
│           (Tier 1 model. Focused prompts.                │
│            Operates on pre-validated code.)               │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Review 1:    │  │ Review 2:    │  │ Review 3:     │  │
│  │ Intent       │─►│ Semantic     │─►│ Risk          │  │
│  │ Alignment    │  │ Capability   │  │ Assessment    │  │
│  │              │  │ Review       │  │               │  │
│  └──────────────┘  └──────────────┘  └───────────────┘  │
└────────────────────────┬────────────────────────────────┘
                         │ ALL PASS
                         ▼
                 ┌───────────────┐
                 │  VERDICT      │
                 │  ASSEMBLER    │
                 └───────┬───────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
          APPROVED   REVISE     ESCALATE
         (→ Executor) (→ Coder)  (→ PM)
```

---

## 3. The Deterministic Layer

These gates run as plain code — Python scripts, AST parsers, regex engines, and existing security tooling. They execute in sequence. **Any single gate failure halts the pipeline** and returns the code to the Coder with a structured error report. The LLM layer never sees code that fails a deterministic check.

### Design Principle

Every deterministic gate produces a **machine-readable verdict**: `PASS`, `FAIL`, or `WARN`. A `FAIL` is a hard stop. A `WARN` is logged and forwarded to the LLM layer for contextual evaluation. This means the deterministic layer is conservative (it can flag false positives) and the LLM layer resolves ambiguity.

---

### Gate 1: Structural Conformance

**Purpose**: Verify that the Coder's output is well-formed and complete — before we even look at what it does.

**What it checks**:

| Check | Method | Verdict on Failure |
|-------|--------|--------------------|
| All declared output files exist | Filesystem check | FAIL |
| Python files parse without syntax errors | `ast.parse()` | FAIL |
| Bash scripts pass `bash -n` (syntax check) | Subprocess | FAIL |
| `requirements.txt` exists if imports detected | AST import scan + file check | FAIL |
| No files outside the declared workroom directory | Path traversal check | FAIL |
| Total output size within limits | Byte count | FAIL |
| File count within limits | Count | FAIL |

**Implementation**:

```python
import ast
import os
import subprocess
from pathlib import Path
from dataclasses import dataclass, field


@dataclass
class GateVerdict:
    gate: str
    status: str  # "PASS", "FAIL", "WARN"
    checks: list[dict] = field(default_factory=list)

    def add_check(self, name: str, passed: bool, detail: str = ""):
        self.checks.append({
            "name": name,
            "passed": passed,
            "detail": detail
        })
        if not passed:
            self.status = "FAIL"

    @property
    def failed_checks(self):
        return [c for c in self.checks if not c["passed"]]


def gate_1_structural_conformance(
    workroom_path: str,
    declared_files: list[str],
    max_total_bytes: int = 10_000_000,  # 10MB
    max_file_count: int = 50
) -> GateVerdict:
    verdict = GateVerdict(gate="structural_conformance", status="PASS")
    workroom = Path(workroom_path).resolve()

    # Check all declared files exist
    for f in declared_files:
        fpath = (workroom / f).resolve()
        if not fpath.exists():
            verdict.add_check("file_exists", False, f"Declared file missing: {f}")
        elif not str(fpath).startswith(str(workroom)):
            verdict.add_check("path_traversal", False, f"Path escapes workroom: {f}")
        else:
            verdict.add_check("file_exists", True, f)

    # Check for undeclared files (Coder produced files it didn't mention)
    actual_files = set()
    for root, dirs, files in os.walk(workroom):
        for fname in files:
            rel = os.path.relpath(os.path.join(root, fname), workroom)
            actual_files.add(rel)

    undeclared = actual_files - set(declared_files)
    if undeclared:
        verdict.add_check(
            "no_undeclared_files", False,
            f"Undeclared files found: {', '.join(sorted(undeclared))}"
        )
    else:
        verdict.add_check("no_undeclared_files", True)

    # Parse Python files
    for f in declared_files:
        if f.endswith(".py"):
            fpath = workroom / f
            if fpath.exists():
                try:
                    source = fpath.read_text()
                    ast.parse(source)
                    verdict.add_check("python_syntax", True, f)
                except SyntaxError as e:
                    verdict.add_check(
                        "python_syntax", False,
                        f"{f}: {e.msg} (line {e.lineno})"
                    )

    # Syntax-check Bash scripts
    for f in declared_files:
        if f.endswith(".sh"):
            fpath = workroom / f
            if fpath.exists():
                result = subprocess.run(
                    ["bash", "-n", str(fpath)],
                    capture_output=True, text=True, timeout=10
                )
                if result.returncode != 0:
                    verdict.add_check(
                        "bash_syntax", False,
                        f"{f}: {result.stderr.strip()}"
                    )
                else:
                    verdict.add_check("bash_syntax", True, f)

    # Size and count limits
    total_bytes = sum(
        os.path.getsize(os.path.join(root, f))
        for root, _, files in os.walk(workroom)
        for f in files
    )
    verdict.add_check(
        "total_size",
        total_bytes <= max_total_bytes,
        f"{total_bytes} bytes (limit: {max_total_bytes})"
    )

    file_count = sum(1 for _ in actual_files)
    verdict.add_check(
        "file_count",
        file_count <= max_file_count,
        f"{file_count} files (limit: {max_file_count})"
    )

    return verdict
```

---

### Gate 2: Secrets & Credential Scan

**Purpose**: Detect hardcoded secrets, API keys, tokens, or credentials anywhere in the Coder's output. This is the most critical deterministic check — a single leaked key can compromise the entire system.

**What it checks**:

| Check | Method | Verdict on Failure |
|-------|--------|--------------------|
| Known secret patterns (AWS, GCP, GitHub, OpenAI, etc.) | Regex library | FAIL |
| High-entropy strings in assignments | Shannon entropy calculation | WARN |
| `os.environ[...]` used correctly (not `os.environ.get("KEY", "sk-actual-key")`) | AST inspection | FAIL |
| No secrets in comments or docstrings | Regex on comment/string extraction | FAIL |
| No credentials in filenames or paths | Filename scan | FAIL |

**Implementation**:

```python
import ast
import math
import re
from collections import Counter


# Comprehensive secret patterns — extend as needed
SECRET_PATTERNS = {
    "aws_access_key":       r'AKIA[A-Z0-9]{16}',
    "aws_secret_key":       r'(?i)aws_secret_access_key\s*[:=]\s*["\']?[A-Za-z0-9/+=]{40}',
    "github_pat":           r'ghp_[A-Za-z0-9]{36}',
    "github_fine_grained":  r'github_pat_[A-Za-z0-9_]{82}',
    "openai_key":           r'sk-[A-Za-z0-9]{32,}',
    "anthropic_key":        r'sk-ant-[A-Za-z0-9\-]{32,}',
    "google_api_key":       r'AIza[A-Za-z0-9_\-]{35}',
    "generic_token":        r'(?i)(api[_-]?key|secret[_-]?key|access[_-]?token|auth[_-]?token)\s*[:=]\s*["\'][A-Za-z0-9_\-]{20,}["\']',
    "private_key_header":   r'-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----',
    "connection_string":    r'(?i)(mongodb|postgres|mysql|redis):\/\/[^\s]{10,}',
    "jwt_token":            r'eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}',
    "slack_token":          r'xox[bporas]-[A-Za-z0-9\-]{10,}',
    "stripe_key":           r'(?:sk|pk)_(test|live)_[A-Za-z0-9]{20,}',
    "sendgrid_key":         r'SG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}',
}

# Known safe patterns that look like secrets but aren't
FALSE_POSITIVE_EXEMPTIONS = [
    r'^["\']?placeholder["\']?$',
    r'^["\']?your[_-]?(api[_-]?)?key[_-]?here["\']?$',
    r'^["\']?TODO["\']?$',
    r'^os\.environ',
]


def shannon_entropy(s: str) -> float:
    """Calculate Shannon entropy of a string."""
    if not s:
        return 0.0
    freq = Counter(s)
    length = len(s)
    return -sum(
        (count / length) * math.log2(count / length)
        for count in freq.values()
    )


def gate_2_secrets_scan(workroom_path: str, files: list[str]) -> GateVerdict:
    verdict = GateVerdict(gate="secrets_scan", status="PASS")
    workroom = Path(workroom_path)

    for fname in files:
        fpath = workroom / fname
        if not fpath.exists():
            continue

        content = fpath.read_text(errors="replace")

        # Pattern matching
        for pattern_name, pattern in SECRET_PATTERNS.items():
            matches = re.finditer(pattern, content)
            for match in matches:
                matched_text = match.group(0)

                # Check against false positive exemptions
                is_exempted = any(
                    re.search(ex, matched_text, re.IGNORECASE)
                    for ex in FALSE_POSITIVE_EXEMPTIONS
                )
                if not is_exempted:
                    # Find line number
                    line_num = content[:match.start()].count('\n') + 1
                    verdict.add_check(
                        f"secret_pattern:{pattern_name}",
                        False,
                        f"{fname}:{line_num} — matches {pattern_name} "
                        f"(redacted: {matched_text[:8]}...)"
                    )

        # High-entropy string detection in Python files
        if fname.endswith(".py"):
            try:
                tree = ast.parse(content)
                for node in ast.walk(tree):
                    if isinstance(node, ast.Constant) and isinstance(node.value, str):
                        s = node.value
                        if len(s) >= 20 and shannon_entropy(s) > 4.5:
                            verdict.add_check(
                                "high_entropy_string",
                                False,  # WARN-level, but we mark as fail for review
                                f"{fname}:{node.lineno} — high-entropy string "
                                f"(len={len(s)}, entropy={shannon_entropy(s):.2f}): "
                                f"'{s[:12]}...'"
                            )
            except SyntaxError:
                pass  # Gate 1 already catches this

        # Check for secrets in comments
        for i, line in enumerate(content.splitlines(), 1):
            stripped = line.strip()
            if stripped.startswith("#") or stripped.startswith("//"):
                for pattern_name, pattern in SECRET_PATTERNS.items():
                    if re.search(pattern, stripped):
                        verdict.add_check(
                            f"secret_in_comment:{pattern_name}",
                            False,
                            f"{fname}:{i} — secret pattern in comment"
                        )

    # Check for env var defaults that contain real-looking values
    for fname in files:
        if not fname.endswith(".py"):
            continue
        fpath = workroom / fname
        if not fpath.exists():
            continue

        try:
            tree = ast.parse(fpath.read_text())
            for node in ast.walk(tree):
                # Catch: os.environ.get("KEY", "actual-secret-value")
                if (isinstance(node, ast.Call)
                    and isinstance(node.func, ast.Attribute)
                    and node.func.attr == "get"
                    and len(node.args) >= 2
                    and isinstance(node.args[1], ast.Constant)
                    and isinstance(node.args[1].value, str)):

                    default_val = node.args[1].value
                    if len(default_val) >= 16 and shannon_entropy(default_val) > 3.5:
                        verdict.add_check(
                            "suspicious_env_default",
                            False,
                            f"{fname}:{node.lineno} — os.environ.get() with "
                            f"high-entropy default (possible leaked credential)"
                        )
        except SyntaxError:
            pass

    if not verdict.failed_checks:
        verdict.add_check("secrets_scan_clean", True, "No secrets detected.")

    return verdict
```

---

### Gate 3: Static Analysis

**Purpose**: Run established security and code quality scanners. These are battle-tested tools that catch vulnerability classes an LLM might miss (SQL injection, path traversal, insecure deserialization, etc.).

**What it runs**:

| Tool | Target | What It Catches | Verdict Mapping |
|------|--------|----------------|-----------------|
| **Bandit** | Python files | Security vulnerabilities (B101-B703) | High-severity → FAIL; Medium → WARN |
| **Semgrep** (optional) | Python/JS/Bash | Custom rules + community rulesets | Configurable per rule |
| **ShellCheck** | Bash scripts | Shell scripting pitfalls | Error-level → FAIL; Warning → WARN |
| **pylint** (security subset) | Python files | Unsafe function calls, eval/exec | FAIL on eval/exec |

**Implementation**:

```python
import json
import subprocess
import shutil


BANDIT_SEVERITY_MAP = {
    "HIGH": "FAIL",
    "MEDIUM": "WARN",
    "LOW": "WARN",
}

# Bandit test IDs that are always FAIL regardless of severity
BANDIT_ALWAYS_FAIL = {
    "B102",  # exec_used
    "B307",  # eval
    "B301",  # pickle
    "B403",  # import_pickle (for review in context)
    "B506",  # yaml_load (unsafe)
    "B602",  # subprocess_popen_with_shell_equals_true
    "B603",  # subprocess_without_shell_equals_true (with user input)
    "B604",  # any_other_function_with_shell_equals_true
    "B608",  # hardcoded_sql_expressions
    "B701",  # jinja2_autoescape_false
}


def gate_3_static_analysis(workroom_path: str, files: list[str]) -> GateVerdict:
    verdict = GateVerdict(gate="static_analysis", status="PASS")
    workroom = Path(workroom_path)

    python_files = [f for f in files if f.endswith(".py")]
    bash_files = [f for f in files if f.endswith(".sh")]

    # --- Bandit ---
    if python_files and shutil.which("bandit"):
        full_paths = [str(workroom / f) for f in python_files]
        try:
            result = subprocess.run(
                ["bandit", "-f", "json", "-r"] + full_paths,
                capture_output=True, text=True, timeout=60,
                cwd=str(workroom)
            )
            if result.stdout:
                bandit_output = json.loads(result.stdout)
                for issue in bandit_output.get("results", []):
                    test_id = issue.get("test_id", "")
                    severity = issue.get("issue_severity", "LOW")
                    confidence = issue.get("issue_confidence", "LOW")

                    # Determine verdict level
                    if test_id in BANDIT_ALWAYS_FAIL:
                        level = "FAIL"
                    elif severity == "HIGH" and confidence in ("HIGH", "MEDIUM"):
                        level = "FAIL"
                    else:
                        level = BANDIT_SEVERITY_MAP.get(severity, "WARN")

                    is_pass = (level != "FAIL")

                    rel_path = os.path.relpath(
                        issue.get("filename", ""), str(workroom)
                    )
                    verdict.add_check(
                        f"bandit:{test_id}",
                        is_pass,
                        f"[{level}] {rel_path}:{issue.get('line_number', '?')} — "
                        f"{issue.get('issue_text', 'Unknown issue')} "
                        f"(severity: {severity}, confidence: {confidence})"
                    )
        except (subprocess.TimeoutExpired, json.JSONDecodeError) as e:
            verdict.add_check(
                "bandit_execution", False,
                f"Bandit failed to execute: {str(e)}"
            )
    elif python_files:
        verdict.add_check(
            "bandit_available", False,
            "Bandit not installed — cannot perform Python security scan."
        )

    # --- ShellCheck ---
    if bash_files and shutil.which("shellcheck"):
        for fname in bash_files:
            fpath = workroom / fname
            try:
                result = subprocess.run(
                    ["shellcheck", "-f", "json", str(fpath)],
                    capture_output=True, text=True, timeout=30
                )
                if result.stdout:
                    issues = json.loads(result.stdout)
                    for issue in issues:
                        level = issue.get("level", "info")
                        is_pass = level not in ("error",)
                        verdict.add_check(
                            f"shellcheck:SC{issue.get('code', '?')}",
                            is_pass,
                            f"{fname}:{issue.get('line', '?')} — "
                            f"[{level}] {issue.get('message', 'Unknown')}"
                        )
            except (subprocess.TimeoutExpired, json.JSONDecodeError) as e:
                verdict.add_check(
                    "shellcheck_execution", False,
                    f"ShellCheck failed on {fname}: {str(e)}"
                )

    # --- AST check for eval/exec/compile (belt-and-suspenders with Bandit) ---
    DANGEROUS_CALLS = {"eval", "exec", "compile", "__import__", "execfile"}

    for fname in python_files:
        fpath = workroom / fname
        if not fpath.exists():
            continue
        try:
            tree = ast.parse(fpath.read_text())
            for node in ast.walk(tree):
                if isinstance(node, ast.Call):
                    call_name = None
                    if isinstance(node.func, ast.Name):
                        call_name = node.func.id
                    elif isinstance(node.func, ast.Attribute):
                        call_name = node.func.attr

                    if call_name in DANGEROUS_CALLS:
                        verdict.add_check(
                            f"dangerous_call:{call_name}",
                            False,
                            f"{fname}:{node.lineno} — "
                            f"Prohibited call to {call_name}()"
                        )
        except SyntaxError:
            pass

    if not verdict.failed_checks:
        verdict.add_check("static_analysis_clean", True, "All scans passed.")

    return verdict
```

---

### Gate 4: Dependency Allowlist

**Purpose**: Verify that every library the code imports is either on the PM's pre-approved list or has been explicitly approved for this task via the Context Packet.

**What it checks**:

| Check | Method | Verdict on Failure |
|-------|--------|--------------------|
| All imports resolve to approved packages | AST import extraction + allowlist lookup | FAIL (unapproved) |
| `requirements.txt` matches actual imports | Cross-reference | WARN |
| No version pinning to known-vulnerable versions | Advisory DB lookup (optional) | WARN |

**Implementation**:

```python
import ast

# The PM maintains this list. It's loaded from a config file, not hardcoded.
# This is an example structure.
DEFAULT_APPROVED_PACKAGES = {
    # Standard library modules are always approved (handled separately)
    # These are third-party packages:
    "requests", "httpx", "aiohttp",
    "pydantic", "jsonschema",
    "click", "typer",
    "pytest",
    "python-dotenv",
}


def extract_imports(source: str) -> set[str]:
    """Extract all top-level package names from import statements."""
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return set()

    imports = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                # 'import foo.bar' → top-level is 'foo'
                imports.add(alias.name.split(".")[0])
        elif isinstance(node, ast.ImportFrom):
            if node.module:
                imports.add(node.module.split(".")[0])
    return imports


def get_stdlib_modules() -> set[str]:
    """Get the set of standard library module names."""
    import sys
    if hasattr(sys, 'stdlib_module_names'):
        return sys.stdlib_module_names  # Python 3.10+
    # Fallback: known common stdlib modules
    return {
        "os", "sys", "json", "re", "math", "pathlib", "typing",
        "collections", "itertools", "functools", "datetime", "time",
        "hashlib", "hmac", "secrets", "uuid", "logging", "argparse",
        "unittest", "dataclasses", "enum", "abc", "io", "csv",
        "subprocess", "shutil", "tempfile", "glob", "fnmatch",
        "urllib", "http", "email", "html", "xml", "sqlite3",
        "configparser", "textwrap", "string", "copy", "pprint",
        "traceback", "warnings", "contextlib", "signal",
        "threading", "multiprocessing", "concurrent", "asyncio",
        "socket", "ssl", "select", "struct", "codecs", "base64",
        "binascii", "pickle", "shelve", "marshal", "zlib", "gzip",
        "bz2", "lzma", "zipfile", "tarfile", "stat",
    }


def gate_4_dependency_allowlist(
    workroom_path: str,
    files: list[str],
    approved_packages: set[str],
    context_packet_deps: list[str],
) -> GateVerdict:
    """
    Args:
        approved_packages: PM's standing approved list.
        context_packet_deps: Packages approved via this task's Context Packet.
    """
    verdict = GateVerdict(gate="dependency_allowlist", status="PASS")
    workroom = Path(workroom_path)
    stdlib = get_stdlib_modules()

    all_approved = approved_packages | set(context_packet_deps) | stdlib

    # Collect all imports across all Python files
    all_imports = set()
    for fname in files:
        if not fname.endswith(".py"):
            continue
        fpath = workroom / fname
        if fpath.exists():
            all_imports |= extract_imports(fpath.read_text(errors="replace"))

    # Check each import
    unapproved = []
    for imp in sorted(all_imports):
        if imp in all_approved:
            verdict.add_check("import_approved", True, imp)
        elif imp.startswith("_"):
            # Internal/private modules — generally stdlib
            verdict.add_check("import_approved", True, f"{imp} (internal)")
        else:
            unapproved.append(imp)
            verdict.add_check(
                "import_approved", False,
                f"Unapproved import: '{imp}' — not on approved list "
                f"and not in Context Packet dependencies."
            )

    # Cross-reference requirements.txt
    req_path = workroom / "requirements.txt"
    if req_path.exists():
        req_packages = set()
        for line in req_path.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#"):
                # Extract package name from 'package>=1.0' style lines
                pkg = re.split(r'[>=<!~\[]', line)[0].strip().lower()
                req_packages.add(pkg)

        # Check for imports not in requirements.txt (and not stdlib)
        third_party_imports = all_imports - stdlib
        for imp in sorted(third_party_imports):
            # Normalize: some packages have different import vs pip names
            imp_lower = imp.lower().replace("_", "-")
            if (imp_lower not in req_packages
                and imp not in req_packages
                and imp.replace("_", "-") not in req_packages):
                verdict.add_check(
                    "import_in_requirements", False,
                    f"Import '{imp}' not found in requirements.txt — "
                    f"will fail at runtime in clean container."
                )

    elif any(i not in stdlib for i in all_imports):
        verdict.add_check(
            "requirements_exists", False,
            "Third-party imports detected but no requirements.txt provided."
        )

    return verdict
```

---

### Gate 5: Capability Fence (AST-Based)

**Purpose**: The mechanical half of "capability leap" detection. Using AST analysis, verify that the code's *observable behaviors* stay within the `permitted_operations` defined in the Context Packet's `constraints` section.

This is the most architecturally interesting gate because it bridges the deterministic and semantic layers. The AST check catches the obvious cases mechanically; the LLM review (Review 2) catches the subtle ones.

**What it checks**:

| Behavior | AST Signal | Maps To Operation |
|----------|-----------|-------------------|
| Opens a file for reading | `open(f, 'r')`, `pathlib.Path.read_text()` | `read_local_file` |
| Opens a file for writing | `open(f, 'w')`, `Path.write_text()` | `write_local_file` |
| Reads an env var | `os.environ[...]`, `os.getenv(...)` | `read_env_var` |
| Makes HTTP request | `requests.get(...)`, `httpx.post(...)` | `http_get`, `http_post`, etc. |
| Executes subprocess | `subprocess.run(...)`, `os.system(...)` | `spawn_subprocess` |
| Opens a socket | `socket.socket(...)` | `network_listen` (usually prohibited) |
| Database operations | `sqlite3.connect(...)`, `cursor.execute(...)` | `sql_read` or `sql_write` |
| Prints to stdout/stderr | `print(...)`, `sys.stdout.write(...)` | `write_stdout` / `write_stderr` |

**Implementation**:

```python
import ast
from typing import Optional


# Maps AST patterns to operation types
OPERATION_DETECTORS = {
    "http_get":          {"requests.get", "httpx.get", "aiohttp.get",
                          "urllib.request.urlopen"},
    "http_post":         {"requests.post", "httpx.post", "aiohttp.post"},
    "http_put":          {"requests.put", "httpx.put"},
    "http_patch":        {"requests.patch", "httpx.patch"},
    "http_delete":       {"requests.delete", "httpx.delete"},
    "read_env_var":      {"os.environ", "os.getenv", "os.environ.get"},
    "spawn_subprocess":  {"subprocess.run", "subprocess.Popen", "subprocess.call",
                          "subprocess.check_output", "subprocess.check_call",
                          "os.system", "os.popen", "os.exec", "os.execvp"},
    "network_listen":    {"socket.socket", "socket.bind", "socket.listen",
                          "socketserver"},
    "write_stdout":      {"print", "sys.stdout.write"},
    "write_stderr":      {"sys.stderr.write"},
}

# File operation detection requires special handling (depends on mode arg)
FILE_OPEN_FUNCTIONS = {"open", "builtins.open", "io.open"}
PATHLIB_READ_METHODS = {"read_text", "read_bytes", "open"}
PATHLIB_WRITE_METHODS = {"write_text", "write_bytes", "mkdir", "touch", "unlink"}
SQL_WRITE_KEYWORDS = {"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE"}


def get_call_name(node: ast.Call) -> Optional[str]:
    """Extract the full dotted name of a function call."""
    if isinstance(node.func, ast.Name):
        return node.func.id
    elif isinstance(node.func, ast.Attribute):
        parts = []
        current = node.func
        while isinstance(current, ast.Attribute):
            parts.append(current.attr)
            current = current.value
        if isinstance(current, ast.Name):
            parts.append(current.id)
        return ".".join(reversed(parts))
    return None


def detect_operations(source: str) -> dict[str, list[dict]]:
    """
    Analyze source code and return detected operations with locations.
    Returns: { "operation_type": [{"line": N, "detail": "..."}, ...] }
    """
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return {}

    detected = {}

    def add_detection(op_type: str, line: int, detail: str):
        if op_type not in detected:
            detected[op_type] = []
        detected[op_type].append({"line": line, "detail": detail})

    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue

        call_name = get_call_name(node)
        if not call_name:
            continue

        # Check standard operation detectors
        for op_type, signatures in OPERATION_DETECTORS.items():
            for sig in signatures:
                if call_name == sig or call_name.endswith(f".{sig}"):
                    add_detection(op_type, node.lineno, f"Call to {call_name}()")
                    break

        # File open detection (depends on mode argument)
        if call_name in FILE_OPEN_FUNCTIONS or call_name.endswith(".open"):
            mode = "r"  # default
            if len(node.args) >= 2 and isinstance(node.args[1], ast.Constant):
                mode = str(node.args[1].value)
            for kw in node.keywords:
                if kw.arg == "mode" and isinstance(kw.value, ast.Constant):
                    mode = str(kw.value.value)

            if any(m in mode for m in ("w", "a", "x", "+")):
                add_detection("write_local_file", node.lineno,
                              f"open() with mode='{mode}'")
            else:
                add_detection("read_local_file", node.lineno,
                              f"open() with mode='{mode}'")

        # Pathlib method detection
        if isinstance(node.func, ast.Attribute):
            if node.func.attr in PATHLIB_READ_METHODS:
                add_detection("read_local_file", node.lineno,
                              f"Path.{node.func.attr}()")
            elif node.func.attr in PATHLIB_WRITE_METHODS:
                add_detection("write_local_file", node.lineno,
                              f"Path.{node.func.attr}()")

        # SQL operation detection (look for .execute() calls with SQL strings)
        if (isinstance(node.func, ast.Attribute)
            and node.func.attr == "execute"
            and node.args
            and isinstance(node.args[0], ast.Constant)
            and isinstance(node.args[0].value, str)):

            sql = node.args[0].value.upper().strip()
            first_word = sql.split()[0] if sql.split() else ""
            if first_word in SQL_WRITE_KEYWORDS:
                add_detection("sql_write", node.lineno,
                              f"SQL {first_word} statement")
            elif first_word == "SELECT":
                add_detection("sql_read", node.lineno, "SQL SELECT statement")

    return detected


def gate_5_capability_fence(
    workroom_path: str,
    files: list[str],
    permitted_operations: list[str],
    prohibited_operations: list[str],
    allowed_write_paths: Optional[list[str]] = None,
    network_allowlist: Optional[list[str]] = None,
) -> GateVerdict:
    verdict = GateVerdict(gate="capability_fence", status="PASS")
    workroom = Path(workroom_path)

    all_detected = {}  # operation_type → list of occurrences across all files

    for fname in files:
        if not fname.endswith(".py"):
            continue
        fpath = workroom / fname
        if not fpath.exists():
            continue

        detected = detect_operations(fpath.read_text(errors="replace"))
        for op_type, occurrences in detected.items():
            tagged = [
                {**occ, "file": fname}
                for occ in occurrences
            ]
            all_detected.setdefault(op_type, []).extend(tagged)

    permitted_set = set(permitted_operations)
    prohibited_set = set(prohibited_operations)

    for op_type, occurrences in all_detected.items():
        if op_type in prohibited_set:
            # Explicitly prohibited — always FAIL
            for occ in occurrences:
                verdict.add_check(
                    f"prohibited_operation:{op_type}",
                    False,
                    f"PROHIBITED operation '{op_type}' detected: "
                    f"{occ['file']}:{occ['line']} — {occ['detail']}"
                )
        elif op_type not in permitted_set:
            # Not on the allowlist — FAIL (capability leap)
            for occ in occurrences:
                verdict.add_check(
                    f"capability_leap:{op_type}",
                    False,
                    f"CAPABILITY LEAP: operation '{op_type}' not in "
                    f"permitted_operations. "
                    f"{occ['file']}:{occ['line']} — {occ['detail']}"
                )
        else:
            # Permitted — PASS
            verdict.add_check(
                f"permitted_operation:{op_type}",
                True,
                f"'{op_type}': {len(occurrences)} occurrence(s)"
            )

    # Verify permitted operations are actually used (informational)
    used_ops = set(all_detected.keys())
    unused_permitted = permitted_set - used_ops
    if unused_permitted:
        # Not a failure, but noted for the LLM review layer
        for op in unused_permitted:
            verdict.add_check(
                f"unused_permission:{op}",
                True,  # Not a failure
                f"NOTE: Permission '{op}' was granted but not used. "
                f"(May indicate over-broad constraints or incomplete implementation.)"
            )

    if not verdict.failed_checks:
        verdict.add_check("capability_fence_clean", True,
                          "All operations within permitted bounds.")

    return verdict
```

---

## 4. The LLM Reasoning Layer

The three LLM reviews run **only on code that has already passed all five deterministic gates**. This means the LLM can focus on semantic and strategic questions rather than mechanical checks. Each review uses a focused, single-purpose prompt to reduce the attack surface and improve reliability.

### Why Three Separate Reviews Instead of One?

A single "review everything" prompt encourages the LLM to satisfice — it'll catch the first problem and declare victory, or spread attention too thin across multiple concerns. Separate, focused reviews ensure each dimension gets full attention. It also enables **independent model selection**: you could run Review 1 on a fast model (Gemini Flash) and reserve Review 2 for a heavyweight (Claude Opus / Gemini Pro).

---

### Review 1: Intent Alignment

**Question**: Does this code actually do what the PM asked for?

**Inputs**:
- The PM's `prd_summary` and `cuj_ids` from the Context Packet
- The Coder's source code
- The Coder's own description of what it built (if provided)

**Prompt Template**:

```
You are a senior product reviewer. Your ONLY job is to determine whether
the submitted code fulfills the Product Manager's stated requirements.

## PM's Requirement (from PRD)
{prd_summary}

## Critical User Journeys This Must Satisfy
{cuj_list}

## The Code
{code_content}

## Your Task
Answer these questions:

1. COMPLETENESS: Does the code address every aspect of the PRD summary?
   List any requirements that are not implemented.

2. CORRECTNESS: Based on the code logic, will it actually produce the
   expected result? Flag any logic errors you can identify.

3. EXCESS: Does the code do anything NOT requested in the PRD?
   This is not necessarily wrong, but must be flagged for review.
   Be specific about what extra functionality exists.

Respond in this exact format:
INTENT_VERDICT: [ALIGNED | PARTIAL | MISALIGNED]
COMPLETENESS_GAPS: [list or "none"]
LOGIC_CONCERNS: [list or "none"]
EXCESS_FUNCTIONALITY: [list or "none"]
SUMMARY: [one paragraph explanation]
```

---

### Review 2: Semantic Capability Review

**Question**: Does this code do anything *beyond* what's mechanically detectable — subtle capability leaps that the AST-based Gate 5 might miss?

This is the review that catches creative workarounds: the Coder that writes data to stdout knowing it'll be piped somewhere, or constructs a URL that exfiltrates data in query parameters, or uses `importlib` to dynamically load a module that bypasses the dependency allowlist.

**Inputs**:
- The full Context Packet (constraints, permitted operations, network allowlist)
- The Coder's source code
- Gate 5's detection report (what operations were mechanically detected)

**Prompt Template**:

```
You are a security auditor specializing in capability analysis. The code
below has already passed automated static analysis and AST-based
capability detection. Your job is to find what the automated tools MISSED.

## Permitted Operations (from Context Packet)
{permitted_operations}

## Prohibited Operations
{prohibited_operations}

## Network Allowlist
{network_allowlist}

## Automated Detection Report (Gate 5)
{gate_5_report}

## The Code
{code_content}

## Your Task
Look for these specific evasion patterns:

1. DYNAMIC IMPORTS: Does the code use importlib, __import__(), or similar
   to load modules at runtime that weren't caught by static analysis?

2. DATA EXFILTRATION VIA ALLOWED CHANNELS: Does the code embed sensitive
   data in URLs, filenames, error messages, or stdout in a way that could
   leak information through permitted operations?

3. CONSTRUCTED OPERATIONS: Does the code build dangerous operations from
   individually-safe parts? (e.g., assembling a shell command as a string,
   then passing it to a function that executes it)

4. SCOPE CREEP VIA DEPENDENCIES: Do any of the imported libraries
   provide capabilities beyond what's needed? (e.g., importing 'requests'
   is fine for HTTP GET, but it also enables POST/PUT/DELETE — does the
   code exploit this?)

5. FILE PATH MANIPULATION: Does the code construct paths that could
   escape the allowed write paths? (symlink attacks, ../../ traversal,
   etc.)

6. TIMING OR SIDE-CHANNEL: Does the code exhibit any behavior that
   depends on timing, external state, or environmental conditions not
   declared in the Context Packet?

For each finding, classify as:
- BLOCK: Must be fixed before execution.
- FLAG: Suspicious but potentially benign — needs PM decision.
- NOTE: Worth documenting but not blocking.

Respond in this exact format:
SEMANTIC_VERDICT: [CLEAN | FINDINGS_PRESENT]
FINDINGS: [list with classification, or "none"]
SUMMARY: [one paragraph]
```

---

### Review 3: Risk Assessment

**Question**: Given everything we know, what's the overall risk profile of executing this code?

This is the synthesis step. It takes the full picture — deterministic gate results, LLM reviews, Context Packet confidence levels — and produces the final recommendation.

**Inputs**:
- All gate verdicts (deterministic layer)
- Review 1 and Review 2 outputs
- Context Packet `researcher_notes` (including confidence levels and caveats)

**Prompt Template**:

```
You are the final risk assessor for a code execution decision. You have
the complete audit trail below. Your job is to synthesize a final verdict.

## Deterministic Gate Results
{gate_results_summary}

## Intent Alignment Review
{review_1_output}

## Semantic Capability Review
{review_2_output}

## Context Packet Confidence & Caveats
{confidence_levels}
{caveats}
{open_questions}

## Your Task
Produce a risk assessment considering:

1. Were there any WARNs from the deterministic layer that, in context,
   should be treated as blocks?

2. Do the EXCESS_FUNCTIONALITY items from Review 1 represent a genuine
   concern or reasonable implementation choices?

3. If Review 2 found FLAGs, are they likely benign or likely concerning
   given the task context?

4. Are there any low-confidence items in the Context Packet that the
   code depends heavily on?

FINAL_VERDICT: [APPROVE | REVISE | ESCALATE_TO_PM]
RISK_LEVEL: [LOW | MEDIUM | HIGH]
CONDITIONS: [any conditions for approval, e.g., "Approve only if PM
confirms single-city scope is sufficient"]
REVISION_ITEMS: [specific items for the Coder to fix, if REVISE]
PM_QUESTIONS: [specific questions for the PM, if ESCALATE]
AUDIT_SUMMARY: [concise paragraph for the audit log]
```

---

## 5. Pipeline Orchestrator

The orchestrator runs the full pipeline and manages the flow between stages.

```python
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from typing import Optional


class PipelineVerdict(Enum):
    APPROVED = "APPROVED"
    REVISE = "REVISE"
    ESCALATE_TO_PM = "ESCALATE_TO_PM"
    DETERMINISTIC_FAIL = "DETERMINISTIC_FAIL"


@dataclass
class AuditRecord:
    """Complete, immutable record of an audit run."""
    audit_id: str
    packet_id: str
    task_id: str
    timestamp: str
    deterministic_gates: list[GateVerdict]
    llm_reviews: list[dict]
    final_verdict: PipelineVerdict
    risk_level: Optional[str] = None
    revision_items: list[str] = field(default_factory=list)
    pm_questions: list[str] = field(default_factory=list)
    audit_summary: str = ""
    total_duration_seconds: float = 0.0


async def run_auditor_pipeline(
    workroom_path: str,
    declared_files: list[str],
    context_packet: dict,
    approved_packages: set[str],
    llm_client,  # Your AG2 / API client for Tier 1 models
) -> AuditRecord:
    """
    Run the full Auditor pipeline on Coder output.

    Returns a complete AuditRecord suitable for logging and forensics.
    """
    start_time = datetime.now(timezone.utc)
    gates = []

    constraints = context_packet.get("constraints", {})
    permitted_ops = constraints.get("permitted_operations", [])
    prohibited_ops = constraints.get("prohibited_operations", [])
    allowed_write_paths = constraints.get("allowed_write_paths")
    network_allowlist = constraints.get("network_allowlist")

    # Extract approved dependency names from Context Packet
    cp_deps = []
    deps = context_packet.get("dependencies", {})
    for lang_deps in deps.values():
        if lang_deps:
            for dep in lang_deps:
                cp_deps.append(dep.get("package", ""))

    # ── DETERMINISTIC LAYER ─────────────────────────────────────────

    # Gate 1: Structural Conformance
    g1 = gate_1_structural_conformance(workroom_path, declared_files)
    gates.append(g1)
    if g1.status == "FAIL":
        return _build_record(
            gates, [], PipelineVerdict.DETERMINISTIC_FAIL,
            context_packet, start_time
        )

    # Gate 2: Secrets Scan
    g2 = gate_2_secrets_scan(workroom_path, declared_files)
    gates.append(g2)
    if g2.status == "FAIL":
        return _build_record(
            gates, [], PipelineVerdict.DETERMINISTIC_FAIL,
            context_packet, start_time
        )

    # Gate 3: Static Analysis
    g3 = gate_3_static_analysis(workroom_path, declared_files)
    gates.append(g3)
    if g3.status == "FAIL":
        return _build_record(
            gates, [], PipelineVerdict.DETERMINISTIC_FAIL,
            context_packet, start_time
        )

    # Gate 4: Dependency Allowlist
    g4 = gate_4_dependency_allowlist(
        workroom_path, declared_files,
        approved_packages, cp_deps
    )
    gates.append(g4)
    if g4.status == "FAIL":
        return _build_record(
            gates, [], PipelineVerdict.DETERMINISTIC_FAIL,
            context_packet, start_time
        )

    # Gate 5: Capability Fence
    g5 = gate_5_capability_fence(
        workroom_path, declared_files,
        permitted_ops, prohibited_ops,
        allowed_write_paths, network_allowlist
    )
    gates.append(g5)
    if g5.status == "FAIL":
        return _build_record(
            gates, [], PipelineVerdict.DETERMINISTIC_FAIL,
            context_packet, start_time
        )

    # ── LLM REASONING LAYER ────────────────────────────────────────

    # Collect code content for LLM reviews
    code_content = _collect_code_content(workroom_path, declared_files)
    gate_5_report = _format_gate_report(g5)

    reviews = []

    # Review 1: Intent Alignment
    r1 = await _run_llm_review(
        llm_client,
        review_type="intent_alignment",
        context_packet=context_packet,
        code_content=code_content,
    )
    reviews.append(r1)

    # Review 2: Semantic Capability Review
    r2 = await _run_llm_review(
        llm_client,
        review_type="semantic_capability",
        context_packet=context_packet,
        code_content=code_content,
        gate_5_report=gate_5_report,
    )
    reviews.append(r2)

    # Review 3: Risk Assessment (synthesis)
    r3 = await _run_llm_review(
        llm_client,
        review_type="risk_assessment",
        context_packet=context_packet,
        code_content=code_content,
        gate_results=gates,
        prior_reviews=reviews,
    )
    reviews.append(r3)

    # Parse final verdict from Review 3
    final_verdict = _parse_final_verdict(r3)

    return _build_record(
        gates, reviews, final_verdict,
        context_packet, start_time
    )


def _build_record(
    gates, reviews, verdict, packet, start_time
) -> AuditRecord:
    elapsed = (datetime.now(timezone.utc) - start_time).total_seconds()
    return AuditRecord(
        audit_id=f"AUDIT-{start_time.strftime('%Y%m%d%H%M%S')}",
        packet_id=packet.get("packet_meta", {}).get("packet_id", "unknown"),
        task_id=packet.get("task_reference", {}).get("task_id", "unknown"),
        timestamp=start_time.isoformat(),
        deterministic_gates=gates,
        llm_reviews=reviews,
        final_verdict=verdict,
        total_duration_seconds=elapsed,
    )
```

---

## 6. Audit Logging and Forensics

Every pipeline run produces an `AuditRecord` that MUST be persisted. This is non-negotiable for a security-first platform.

### What Gets Logged

| Field | Purpose | Retention |
|-------|---------|-----------|
| `audit_id` | Unique run identifier | Permanent |
| `packet_id` | Links to the Context Packet that was audited against | Permanent |
| `task_id` | Links to the PM's original task | Permanent |
| `timestamp` | When the audit ran | Permanent |
| `deterministic_gates` | Full verdicts from all 5 gates, including check details | Permanent |
| `llm_reviews` | Full text of all 3 LLM review responses | Permanent |
| `final_verdict` | APPROVED / REVISE / ESCALATE / DETERMINISTIC_FAIL | Permanent |
| `total_duration_seconds` | Pipeline performance metric | Permanent |
| Code snapshot (hash) | SHA-256 of the workroom at audit time | Permanent |

### What Does NOT Get Logged

- Actual secret values (only pattern match locations)
- Raw LLM system prompts (stored separately in config management)
- User identity beyond what the PM provides

### Storage

For v1, append-only JSON Lines (`.jsonl`) file is sufficient. Each line is a complete `AuditRecord` serialized to JSON. For production, migrate to an immutable log store (e.g., append-only S3 bucket with object lock, or a dedicated audit database with write-once semantics).

---

## 7. Circuit Breakers and Failure Modes

### Deterministic Failure → Coder Revision

When any gate in the deterministic layer fails, the pipeline returns immediately with `DETERMINISTIC_FAIL`. The error report is structured for the Coder to act on:

```json
{
  "verdict": "DETERMINISTIC_FAIL",
  "failed_gate": "capability_fence",
  "failures": [
    {
      "check": "capability_leap:spawn_subprocess",
      "detail": "main.py:42 — Call to subprocess.run(). Operation 'spawn_subprocess' not in permitted_operations.",
      "fix_guidance": "Remove the subprocess call. If you need to execute an external command, escalate to the PM to request 'spawn_subprocess' permission."
    }
  ],
  "passing_gates": ["structural_conformance", "secrets_scan", "static_analysis", "dependency_allowlist"]
}
```

### LLM Review → Revision or Escalation

The LLM layer can return three verdicts:

- **APPROVE**: All clear. Proceed to Executor.
- **REVISE**: Specific issues the Coder can fix. Returns to the Coder with actionable items.
- **ESCALATE_TO_PM**: Questions that require human judgment. The PM decides.

### Loop Limits

| Scenario | Limit | Action on Breach |
|----------|-------|-----------------|
| Coder → Auditor deterministic failures | 3 attempts | Escalate task to PM with full failure history |
| Coder → Auditor LLM revision requests | 2 attempts | Escalate to PM |
| Total pipeline runs per task | 5 | Hard stop. PM must manually review and either redefine the task or approve an override. |
| Auditor LLM review timeout | 120 seconds per review | Treat as ESCALATE (fail-safe, not fail-open) |
| Auditor LLM returns unparseable response | 1 retry | Treat as ESCALATE |

**Critical principle**: All failure modes default to **ESCALATE**, never to **APPROVE**. The system fails closed.

---

## 8. Compute Tier Assignment

Mapping to your Tiered Compute architecture:

| Component | Tier | Rationale |
|-----------|------|-----------|
| Gates 1-5 (Deterministic) | **Tier 2 (Local)** | Pure computation. Run on your NVIDIA boxes. Fast, free, no network needed. |
| Review 1 (Intent Alignment) | **Tier 1 (Cloud)** | Requires strong reasoning about product requirements. |
| Review 2 (Semantic Capability) | **Tier 1 (Cloud)** | Security-critical reasoning. Use the strongest available model. |
| Review 3 (Risk Assessment) | **Tier 1 (Cloud)** | Synthesis across multiple inputs. Strongest model. |
| Audit Log Persistence | **Tier 2 (Local)** | Sensitive data stays on your hardware. |

This means your typical audit run hits the cloud for only 3 LLM calls, while the 5 deterministic gates run locally at zero token cost. For tasks where the Coder gets it right on the first try, that's the entire cloud cost of the audit layer.

---

## 9. Future Enhancements

### Dual-Model Verification for Review 2

For the most security-critical review (Semantic Capability), run it on TWO different models (e.g., Claude + Gemini) and require agreement. If they disagree, escalate. This defends against model-specific blind spots.

### Adaptive Gate Configuration

As the platform accumulates audit history, analyze which gates catch issues most frequently for different task types. Use this data to configure gate strictness per task category (e.g., "API integration" tasks get stricter network allowlist checks; "data transformation" tasks get stricter file path checks).

### Red Team Test Suite

Maintain a library of deliberately malicious code submissions that exercise each gate and review. Run these as regression tests whenever the pipeline is updated. Categories:

- Hardcoded secrets (various provider formats)
- Dynamic import evasion
- Path traversal attacks
- Data exfiltration via allowed channels
- Prompt injection in code comments
- Capability leap via dependency side-effects
