"""Context Packet schema validator for the perry audit pipeline.

Reads JSON from stdin: {"packet": {...}}
Writes JSON to stdout: {"pass": bool, "gate": "schema_validation", "findings": [...]}
"""
import json
import os
import sys

import jsonschema


def validate(packet: dict) -> dict:
    schema_path = os.path.join(os.path.dirname(__file__), "schema.json")
    with open(schema_path) as f:
        schema = json.load(f)

    validator = jsonschema.Draft7Validator(schema)
    findings = []
    for error in sorted(validator.iter_errors(packet), key=str):
        path = ".".join(str(p) for p in error.absolute_path)
        if path:
            findings.append(f"{path}: {error.message}")
        else:
            findings.append(error.message)

    return {
        "pass": len(findings) == 0,
        "gate": "schema_validation",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = validate(input_data["packet"])
    print(json.dumps(result))
