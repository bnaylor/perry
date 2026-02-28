# python/audit/test_ast_analyzer.py
import json
import subprocess
import sys

def run_analyzer(code, allowed_imports=None):
    """Run ast_analyzer.py as a subprocess and return parsed JSON."""
    input_data = json.dumps({
        "code": code,
        "allowed_imports": allowed_imports or []
    })
    result = subprocess.run(
        [sys.executable, "python/audit/ast_analyzer.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def test_clean_code_passes():
    result = run_analyzer(
        "import json\nx = json.loads('{}')",
        allowed_imports=["json"]
    )
    assert result["pass"] is True
    assert result["findings"] == []

def test_disallowed_import_fails():
    result = run_analyzer(
        "import os\nos.system('rm -rf /')",
        allowed_imports=["json", "requests"]
    )
    assert result["pass"] is False
    assert any("os" in f for f in result["findings"])

def test_subprocess_import_fails():
    result = run_analyzer(
        "import subprocess\nsubprocess.run(['ls'])",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("subprocess" in f for f in result["findings"])

def test_eval_call_detected():
    result = run_analyzer(
        "x = eval('1+1')",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("eval" in f for f in result["findings"])

def test_exec_call_detected():
    result = run_analyzer(
        "exec('print(1)')",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("exec" in f for f in result["findings"])

def test_from_import_checked():
    result = run_analyzer(
        "from os.path import join",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("os" in f for f in result["findings"])
