#!/usr/bin/env python3
"""Package the single maintained Docker configuration; no generated config or binaries."""
import argparse
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', type=Path, default=root / 'dist/multiharness-docker.zip')
args = parser.parse_args()
args.output.parent.mkdir(parents=True, exist_ok=True)
with ZipFile(args.output, 'w', compression=ZIP_DEFLATED) as archive:
    for name in ('compose.yaml', '.env.example', 'docker/seccomp.json',
                 'docker/compose.linux.yaml', 'docker/apparmor.profile',
                 'docker/NOTICE.md', 'docker/LICENSE.moby',
                 'scripts/magent-apparmor.sh', 'scripts/setup.sh', 'scripts/setup.ps1', 'docs/docker.md'):
        archive.write(root / name, name)
print(args.output)
