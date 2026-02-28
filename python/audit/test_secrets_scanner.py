# python/audit/test_secrets_scanner.py
import json
import subprocess
import sys

def run_scanner(code):
    input_data = json.dumps({"code": code})
    result = subprocess.run(
        [sys.executable, "python/audit/secrets_scanner.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def test_clean_code_passes():
    result = run_scanner("x = 42\nprint(x)")
    assert result["pass"] is True

def test_aws_key_detected():
    result = run_scanner("key = 'AKIAIOSFODNN7EXAMPLE'")
    assert result["pass"] is False
    assert any("AWS" in f or "AKIA" in f for f in result["findings"])

def test_github_token_detected():
    result = run_scanner("token = 'ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234'")
    assert result["pass"] is False

def test_generic_api_key_detected():
    result = run_scanner("API_KEY = 'sk-abcdefghijklmnopqrstuvwxyz123456'")
    assert result["pass"] is False

def test_env_var_reference_is_fine():
    result = run_scanner("key = os.environ['API_KEY']")
    assert result["pass"] is True

def test_bearer_token_detected():
    result = run_scanner("headers = {'Authorization': 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc123'}")
    assert result["pass"] is False
