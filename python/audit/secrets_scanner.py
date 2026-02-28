"""Secrets and credential scanner for the perry audit pipeline.

Reads JSON from stdin: {"code": "..."}
Writes JSON to stdout: {"pass": bool, "gate": "secrets", "findings": [...]}
"""
import json
import re
import sys

PATTERNS = [
    (r"(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*[\"']?[a-zA-Z0-9_\-]{20,}", "possible hardcoded credential"),
    (r"(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}", "possible bearer token"),
    (r"ghp_[a-zA-Z0-9]{36}", "GitHub personal access token"),
    (r"sk-[a-zA-Z0-9]{32,}", "OpenAI-style API key"),
    (r"AIza[a-zA-Z0-9_\-]{35}", "Google API key"),
    (r"AKIA[A-Z0-9]{16}", "AWS access key"),
]


def scan(code: str) -> dict:
    findings = []
    for line_num, line in enumerate(code.splitlines(), 1):
        for pattern, description in PATTERNS:
            if re.search(pattern, line):
                findings.append(f"line {line_num}: {description}")
                break  # one finding per line is enough

    return {
        "pass": len(findings) == 0,
        "gate": "secrets",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = scan(input_data["code"])
    print(json.dumps(result))
