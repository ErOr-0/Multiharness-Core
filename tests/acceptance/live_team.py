"""Opt-in live Team check: all three roles use the user's configured native agents."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--binary", required=True)
parser.add_argument("--config", required=True)
args = parser.parse_args()
assert not os.environ.get("CI"), "Live account tests must be selected locally"
settings_path = Path(args.config)
before = settings_path.read_bytes()
settings = json.loads(before)
env = {k: v for k, v in os.environ.items() if not k.startswith(("MULTIHARNESS_", "MAGENT_", "BDD_"))}
env["PYTHONDONTWRITEBYTECODE"] = "1"
with tempfile.TemporaryDirectory(prefix="magent-live-team-") as temp:
    root = Path(temp)
    project = root / "project"
    project.mkdir()
    (project / "calc.py").write_text("def add(a, b):\n    return a - b\n")
    settings.update(mode="team", working_dir=str(project), timeout="6m", max_repair_attempts=0)
    settings.setdefault("workspace", {})["recovery_dir"] = str(root / "recovery")
    check = "from calc import add; assert add(2,3)==5; assert add(-2,3)==1; assert add(0,0)==0; print('independent validation passed')"
    settings["validation"] = {"checks": [{"executable": "python3", "args": ["-B", "-c", check]}]}
    config = root / "config.json"
    config.write_text(json.dumps(settings))
    try:
        p = subprocess.run([args.binary, "--config", str(config), "--workdir", str(project),
                            "--install-mode", "disabled", "--progress", "plain", "--task",
                            "Fix calc.py so add(a, b) returns a + b. Only edit that file. "
                            "Inspect it, make the one-line change, and use the configured validation. "
                            "Do not add architecture, dependencies or other files. If running Python checks "
                            "use python3 -B to avoid generating bytecode files."],
                           cwd=project, env=env, capture_output=True, text=True, timeout=380)
        result = json.loads(p.stdout)
        assert p.returncode == 0 and result["status"] == "approved", p.stdout + p.stderr
        assert result["agent_invocations"] == 3 and result["repair_attempts"] == 0, result
        assert result["validation"]["passed"] and result["last_review"]["approved"], result
        assert result["repository"]["changed_files"] == ["calc.py"], result
        subprocess.run(["python3", "-B", "-c", check], cwd=project, check=True, timeout=20)
        assert config.read_text() == json.dumps(settings), "Test role preferences changed"
        print("PASS: configured native planner, implementer and reviewer completed a real edit and independent validation")
    finally:
        assert settings_path.read_bytes() == before, "Saved user configuration changed"
