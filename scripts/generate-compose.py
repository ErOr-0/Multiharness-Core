#!/usr/bin/env python3
"""Generate downloadable Compose configuration from the audited policy."""
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
profile = json.loads((root / 'docker/seccomp.json').read_text())
service = {
    'image': 'er0r2/multiharness-core:preview',
    'init': True,
    'stdin_open': True,
    'tty': True,
    'user': '${MAGENT_UID:-1000}:${MAGENT_GID:-1000}',
    'cap_drop': ['ALL'],
    'security_opt': ['no-new-privileges=true', 'seccomp=./seccomp.json'],
    'volumes': [
        {'type': 'bind', 'source': '__MAGENT_PROJECT_DIR__', 'target': '/workspace',
         'bind': {'create_host_path': False}},
        {'type': 'volume', 'source': 'state', 'target': '/state'},
    ],
}
document = {'name': 'multiharness', 'services': {'magent': service},
            'volumes': {'state': {'name': '${MAGENT_STATE_VOLUME:-magent-state}'}}}
header = '# Multiharness: original host files at /workspace; logins/settings in magent-state.\n'
header += '# JSON is valid YAML. Keep seccomp.json beside this file; see NOTICE.md.\n'
header += '# Set the bind source to your existing absolute host folder. Do not use compose down -v.\n'
content = header + json.dumps(document, indent=2) + '\n'
(root / 'web/public/compose.yaml').write_text(content, encoding='utf-8')
(root / 'web/public/seccomp.json').write_text(json.dumps(profile, indent=2) + '\n', encoding='utf-8')
(root / 'web/public/NOTICE.md').write_bytes((root / 'docker/NOTICE.md').read_bytes())
(root / 'web/public/LICENSE.moby').write_bytes((root / 'docker/LICENSE.moby').read_bytes())
