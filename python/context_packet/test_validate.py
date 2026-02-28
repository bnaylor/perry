# python/context_packet/test_validate.py
import json
import subprocess
import sys
import os

SCHEMA_PATH = os.path.join(os.path.dirname(__file__), "schema.json")

def run_validator(packet):
    input_data = json.dumps({"packet": packet})
    result = subprocess.run(
        [sys.executable, "python/context_packet/validate.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def minimal_valid_packet():
    return {
        "packet_meta": {
            "packet_id": "CP-20260227-a3f8c012",
            "schema_version": "1.0.0",
            "created_at": "2026-02-27T14:30:00Z",
            "researcher_model": "gemini-2.5-pro",
            "source_count": 1,
            "parent_packet_id": None,
        },
        "task_reference": {
            "task_id": "TASK-001",
            "prd_summary": "Test task",
            "cuj_ids": ["CUJ-001"],
        },
        "external_apis": [],
        "data_schemas": [],
        "constraints": {
            "permitted_operations": ["write_stdout"],
            "prohibited_operations": ["network_listen"],
        },
        "researcher_notes": {
            "summary": "Test summary.",
            "open_questions": [],
        },
    }

def test_valid_packet_passes():
    result = run_validator(minimal_valid_packet())
    assert result["pass"] is True

def test_missing_required_field_fails():
    packet = minimal_valid_packet()
    del packet["task_reference"]
    result = run_validator(packet)
    assert result["pass"] is False
    assert any("task_reference" in f for f in result["findings"])

def test_invalid_packet_id_format_fails():
    packet = minimal_valid_packet()
    packet["packet_meta"]["packet_id"] = "bad-id"
    result = run_validator(packet)
    assert result["pass"] is False

def test_invalid_schema_version_fails():
    packet = minimal_valid_packet()
    packet["packet_meta"]["schema_version"] = "2.0.0"
    result = run_validator(packet)
    assert result["pass"] is False
