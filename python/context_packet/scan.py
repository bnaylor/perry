"""Content scanner for Context Packets — checks for secrets and injection patterns.

Reads JSON from stdin: {"packet": {...}}
Writes JSON to stdout: {"pass": bool, "gate": "content_scan", "findings": [...]}
"""
import json
import re
import sys

SECRETS_PATTERNS = [
    r"(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*[\"']?[a-zA-Z0-9_\-]{20,}",
    r"(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}",
    r"ghp_[a-zA-Z0-9]{36}",
    r"sk-[a-zA-Z0-9]{32,}",
    r"AIza[a-zA-Z0-9_\-]{35}",
    r"AKIA[A-Z0-9]{16}",
]

INJECTION_PATTERNS = [
    r"(?i)ignore\s+(previous|above|all)\s+(instructions?|prompts?)",
    r"(?i)you\s+are\s+now\s+",
    r"(?i)system\s*:\s*",
    r"(?i)forget\s+(everything|your|all)",
    r"(?i)<\s*/?system\s*>",
]


def scan(packet: dict) -> dict:
    findings = []

    def walk(obj, path="root"):
        if isinstance(obj, str):
            for pattern in SECRETS_PATTERNS:
                if re.search(pattern, obj):
                    findings.append(f"SECRETS_LEAK at {path}")
                    break
            for pattern in INJECTION_PATTERNS:
                if re.search(pattern, obj):
                    findings.append(f"INJECTION_SUSPECT at {path}")
                    break
        elif isinstance(obj, dict):
            for k, v in obj.items():
                walk(v, f"{path}.{k}")
        elif isinstance(obj, list):
            for i, v in enumerate(obj):
                walk(v, f"{path}[{i}]")

    walk(packet)
    return {
        "pass": len(findings) == 0,
        "gate": "content_scan",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = scan(input_data["packet"])
    print(json.dumps(result))
