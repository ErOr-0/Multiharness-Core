#!/usr/bin/env python3
"""Build native Docker launchers for the website and verify archive checksums."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

root = Path(__file__).resolve().parents[1]
version = sys.argv[1] if len(sys.argv) > 1 else "preview"
output = root / "web/public/downloads"
output.mkdir(parents=True, exist_ok=True)
manifest = {}
for system in ("windows", "darwin", "linux"):
    for arch in ("amd64", "arm64"):
        name = "magent.exe" if system == "windows" else "magent"
        archive = output / f"magent-host_{system}_{arch}.zip"
        with tempfile.TemporaryDirectory(prefix="magent-host-build-") as temporary:
            binary = Path(temporary) / name
            env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0")
            subprocess.run(["go", "build", "-trimpath", "-ldflags", f"-s -w -X main.version={version}", "-o", str(binary), "./cmd/magent-launcher"], cwd=root, env=env, check=True)
            with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as package:
                info = zipfile.ZipInfo(name)
                info.external_attr = 0o100755 << 16
                package.writestr(info, binary.read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
                package.write(root / "docs/host-launcher.md", "README.md")
                package.write(root / "docker/NOTICE.md", "NOTICE.md")
                package.write(root / "docker/LICENSE.moby", "LICENSE.moby")
                if system == "linux":
                    package.write(root / "docker/apparmor.profile", "docker/apparmor.profile")
                    package.write(root / "scripts/magent-apparmor.sh", "scripts/magent-apparmor.sh")
            with zipfile.ZipFile(archive) as package:
                assert package.testzip() is None
        manifest[archive.name] = hashlib.sha256(archive.read_bytes()).hexdigest()
        print("Built", archive.name)
(output / "checksums.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
