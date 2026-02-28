"""AST-based code analyzer for the perry audit pipeline.

Reads JSON from stdin: {"code": "...", "allowed_imports": ["json", ...]}
Writes JSON to stdout: {"pass": bool, "gate": "ast", "findings": [...]}
"""
import ast
import json
import sys

DANGEROUS_CALLS = {"eval", "exec", "compile", "__import__"}


def analyze(code: str, allowed_imports: list[str]) -> dict:
    findings = []

    try:
        tree = ast.parse(code)
    except SyntaxError as e:
        return {"pass": False, "gate": "ast", "findings": [f"SyntaxError: {e}"]}

    for node in ast.walk(tree):
        # Check imports
        if isinstance(node, ast.Import):
            for alias in node.names:
                root_module = alias.name.split(".")[0]
                if root_module not in allowed_imports:
                    findings.append(f"disallowed import: {alias.name}")

        elif isinstance(node, ast.ImportFrom):
            if node.module:
                root_module = node.module.split(".")[0]
                if root_module not in allowed_imports:
                    findings.append(f"disallowed import: from {node.module}")

        # Check dangerous function calls
        elif isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id in DANGEROUS_CALLS:
                findings.append(f"dangerous call: {node.func.id}()")

    return {
        "pass": len(findings) == 0,
        "gate": "ast",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = analyze(input_data["code"], input_data.get("allowed_imports", []))
    print(json.dumps(result))
