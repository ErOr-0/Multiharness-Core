#!/usr/bin/env python3
"""Exercise setup prompts/persistence using a fake Docker CLI; create no containers."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix='multiharness-setup-test-') as scratch:
    scratch = Path(scratch)
    bundle = scratch / 'configuration'
    shutil.copytree(root / 'docker', bundle / 'docker')
    (bundle / 'scripts').mkdir()
    shutil.copy(root / 'compose.yaml', bundle / 'compose.yaml')
    shutil.copy(root / 'scripts/setup.sh', bundle / 'scripts/setup.sh')
    project = scratch / "User's $project with spaces"
    project.mkdir()
    fake = scratch / 'bin'
    fake.mkdir()
    log = scratch / 'docker.log'
    (fake / 'docker').write_text('''#!/usr/bin/env python3
import json,os,sys
with open(os.environ['SETUP_TEST_LOG'],'a') as f:f.write(json.dumps(sys.argv[1:])+'\\n')
if sys.argv[1]=='inspect': print(os.environ.get('SETUP_TEST_RUNNING','false'))
if sys.argv[1]=='info' and '--format' in sys.argv: print('[]')
''')
    (fake / 'docker').chmod(0o755)
    env = dict(os.environ, PATH=str(fake)+os.pathsep+os.environ['PATH'], SETUP_TEST_LOG=str(log))
    def run(lines='', success=True):
        p = subprocess.run(['bash', str(bundle / 'scripts/setup.sh')], input=lines,
                           text=True, capture_output=True, env=env, cwd='/', timeout=10)
        assert (p.returncode == 0) == success, p.stdout+p.stderr
        return p.stdout
    run(str(scratch / 'missing')+'\n', success=False)
    assert not (bundle / '.env').exists()
    assert 'Folder saved.' in run(str(project)+'\n')
    saved = (bundle / '.env').read_bytes()
    assert 'Using your saved setup.' in run()
    assert (bundle / '.env').read_bytes() == saved
    calls = [json.loads(line) for line in log.read_text().splitlines()]
    assert sum(call[-2:]==['up','--no-start'] for call in calls)==2
    assert sum(call==['start','-ai','multiharness'] for call in calls)==2
    env['SETUP_TEST_RUNNING']='true'
    before=log.read_text()
    assert 'already running' in run()
    assert all('up' not in json.loads(line) and 'pull' not in json.loads(line) for line in log.read_text()[len(before):].splitlines())
    # Parse the saved path using real Compose if installed. Config is read-only.
    docker = shutil.which('docker')
    if docker:
        clean = {k:v for k,v in os.environ.items() if not k.startswith('MULTIHARNESS_')}
        output=subprocess.check_output([docker,'compose','--env-file',str(bundle/'.env'),'-f',str(bundle/'compose.yaml'),'config','--format','json'],text=True,env=clean)
        config=json.loads(output)
        assert config['services']['multiharness']['volumes'][0]['source'].replace('$$','$')==str(project.resolve())
    print('PASS: first-run prompt, literal path persistence, repeat setup and running-session protection; no containers created')
