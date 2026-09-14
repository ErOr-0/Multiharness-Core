import os
from pathlib import Path
import subprocess
import tempfile


def before_all(context):
    context.repo = Path(__file__).resolve().parents[3]
    context.build = tempfile.TemporaryDirectory(prefix="magent-bdd-build-")
    context.add_cleanup(context.build.cleanup)
    suffix = ".exe" if os.name == "nt" else ""
    selected = context.config.userdata.get("binary")
    context.binary = str(Path(selected).resolve()) if selected else str(Path(context.build.name) / ("magent" + suffix))
    context.fixture = str(Path(context.build.name) / ("provider-fixture" + suffix))
    context.build_state = {"binary_needed": not selected, "fixture_built": False}


def before_scenario(context, scenario):
    context.scratch = tempfile.TemporaryDirectory(prefix="magent-bdd-")
    context.add_cleanup(context.scratch.cleanup)
    context.root = Path(context.scratch.name)
    context.workspace = context.root / "workspace with spaces"
    context.workspace.mkdir()
    context.env = {k: v for k, v in os.environ.items() if not k.startswith(("MULTIHARNESS_", "MAGENT_", "BDD_"))}
    context.env["MULTIHARNESS_RUNTIME_CHECK"] = "0"
    context.env["MULTIHARNESS_INSTALL_MODE"] = "disabled"
    context.env["BDD_RECORD"] = str(context.root / "calls.jsonl")
    context.env["BDD_BEHAVIOR"] = "success"
    context.config_path = context.root / "config.json"
    context.overrides = []
