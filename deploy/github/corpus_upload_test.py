import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import tempfile
import unittest

from corpus_test_support import save_fixture, write_fixture

ROOT = Path(__file__).resolve().parents[2]
FAKE = '''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import sys
import hashlib
args = sys.argv[1:]
name = Path(sys.argv[0]).name
build = Path(os.environ['FAKE_BUILD'])
remote = Path(os.environ.get('FAKE_REMOTE', os.environ['FAKE_BUILD']))
def option(key):
    return args[args.index(key) + 1]
def etag(path):
    return '"' + hashlib.sha256(path.read_bytes()).hexdigest() + '"'
def block_upload(filename):
    if filename == 'corpus.dump' and os.getenv('FAKE_READY'):
        with open(os.environ['FAKE_READY'], 'w') as stream:
            stream.write('ready')
        with open(os.environ['FAKE_RESUME']) as stream:
            stream.read(1)
with open(os.environ['FAKE_LOG'], 'a') as stream:
    stream.write(json.dumps(dict(name=name, args=args, checksum=os.getenv('AWS_REQUEST_CHECKSUM_CALCULATION'),
                                run=os.getenv('FAKE_RUN', ''),
                                config=Path(os.environ['AWS_CONFIG_FILE']).read_text() if 'AWS_CONFIG_FILE' in os.environ else '')) + '\\n')
if name == 'oras':
    manifest = (build / 'previous-manifest.json').read_bytes()
    signature = b'synthetic bundle'
    if 'resolve' in args:
        print('sha256:' + 'c' * 64)
    elif 'manifest' in args:
        print(json.dumps(dict(artifactType='application/vnd.trachuan.corpus-release.v1',
                              layers=[dict(digest='sha256:' + hashlib.sha256(data).hexdigest(), size=len(data),
                                           annotations={'org.opencontainers.image.title': title})
                                      for title, data in [('manifest.json', manifest), ('manifest.sigstore.json', signature)]])))
    elif 'blob' in args:
        sys.stdout.buffer.write(manifest if args[-1].endswith(hashlib.sha256(manifest).hexdigest()) else signature)
elif name == 'cosign':
    if os.getenv('FAKE_BAD_SIGNATURE'):
        sys.exit(1)
elif '--version' in args:
    print('aws-cli/' + os.getenv('FAKE_AWS_VERSION', '2.31.0'))
elif 'list-objects-v2' in args:
    count = sum(path.is_file() for path in remote.iterdir()) if remote.exists() else 0
    print(os.getenv('FAKE_KEY_COUNT', str(count)))
elif 'head-object' in args:
    filename = option('--key').rsplit('/', 1)[1]
    path = build / 'previous-report.json' if option('--key').startswith('vn/20990101_1/') else remote / filename
    if not path.exists():
        path = build / filename
    size = json.loads(os.getenv('FAKE_HEAD_SIZES', '{}')).get(filename, path.stat().st_size)
    result = dict(ContentLength=size + (1 if os.getenv('FAKE_BAD_SIZE') == filename else 0), ETag=etag(path))
    result['ETag'] = json.loads(os.getenv('FAKE_HEAD_ETAGS', '{}')).get(filename, result['ETag'])
    print(result['ContentLength'] if '--query' in args else json.dumps(result))
elif 'put-object' in args:
    filename = option('--key').rsplit('/', 1)[1]
    block_upload(filename)
    if os.getenv('FAKE_FAIL_UPLOAD'):
        sys.exit(1)
    remote.mkdir(exist_ok=True)
    destination = remote / filename
    shutil.copyfile(option('--body'), destination)
    result = dict(ETag=etag(destination))
    print(json.dumps(result))
    if filename == 'release.json' and os.getenv('FAKE_REPLACE_AFTER_UPLOAD'):
        replaced = remote / os.environ['FAKE_REPLACE_AFTER_UPLOAD']
        replaced.write_bytes(b'x' * replaced.stat().st_size)
elif 'get-object' in args:
    filename = option('--key').rsplit('/', 1)[1]
    source = build / ('previous-report.json' if option('--key').startswith('vn/20990101_1/') else filename)
    data = source.read_bytes()
    if os.getenv('FAKE_GROW_AFTER_HEAD') == filename:
        data += b'growth'
    total = len(data)
    object_etag = '"' + hashlib.sha256(data).hexdigest() + '"'
    first, last = '0', str(total - 1)
    if '--range' in args and os.getenv('FAKE_IGNORE_RANGE') != filename:
        first, last = option('--range').removeprefix('bytes=').split('-')
        data = data[int(first):int(last) + 1]
    Path(args[-1]).write_bytes(data)
    result = dict(ContentLength=len(data), ETag=object_etag, ContentRange=f'bytes {first}-{last}/{total}')
    result.update(json.loads(os.getenv('FAKE_GET_METADATA', '{}')).get(filename, {}))
    print(json.dumps(result))
elif name == 'aws' and 'cp' in args:
    index = args.index('cp')
    if args[index + 1].startswith('s3://'):
        data = (build / args[index + 1].rsplit('/', 1)[1]).read_bytes()
        if os.getenv('FAKE_GROW_AFTER_HEAD'):
            data += b'growth'
        Path(args[index + 2]).write_bytes(data)
    else:
        remote.mkdir(exist_ok=True)
        destination = remote / args[index + 2].rsplit('/', 1)[1]
        block_upload(destination.name)
        shutil.copyfile(args[index + 1], destination)
        if destination.name == 'release.json' and os.getenv('FAKE_REPLACE_AFTER_UPLOAD'):
            replaced = remote / os.environ['FAKE_REPLACE_AFTER_UPLOAD']
            replaced.write_bytes(b'x' * replaced.stat().st_size)
    if os.getenv('FAKE_FAIL_UPLOAD'):
        sys.exit(1)
elif name == 'gh':
    if os.getenv('FAKE_FAIL_DISPATCH'):
        sys.exit(1)
else:
    sys.exit('unsupported synthetic client call')
'''


class CorpusUploadTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.build = self.path / 'build with spaces'
        self.build.mkdir()
        self.row, self.report = write_fixture(self.build)
        self.bin = self.path / 'bin'
        self.bin.mkdir()
        for name in ('aws', 'gh', 'oras', 'cosign'):
            executable = self.bin / name
            executable.write_text(FAKE)
            executable.chmod(0o755)
        self.log = self.path / 'calls.jsonl'
        self.remote = self.path / 'remote'
        self.remote.mkdir()
        self.script = self.checkout_script('repo')
        self.env = dict(PATH=str(self.bin) + os.pathsep + os.environ['PATH'], HOME=str(self.path),
                        OCI_S3_ENDPOINT='https://synthetic.compat.objectstorage.ap-singapore-1.oraclecloud.com',
                        FAKE_LOG=str(self.log), FAKE_BUILD=str(self.build), FAKE_REMOTE=str(self.remote),
                        PYTHONDONTWRITEBYTECODE='1')

    def checkout_script(self, name):
        repo = self.path / name
        (repo / 'scripts').mkdir(parents=True)
        script = repo / 'scripts/corpus-upload.sh'
        shutil.copyfile(ROOT / 'scripts/corpus-upload.sh', script)
        (repo / 'deploy').symlink_to(ROOT / 'deploy', target_is_directory=True)
        return script

    def upload_command(self, *flags, script=None):
        return ['bash', str(script or self.script), *flags, 'vn', '20990102_1',
                str(self.build), str(self.build / 'report.json')]

    def run_upload(self, *flags):
        return subprocess.run(self.upload_command(*flags),
                              env=self.env, capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_uploads_four_objects_verifies_sizes_then_dispatches_main(self):
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        uploads = [call for call in calls if 'put-object' in call['args']]
        self.assertEqual(len(uploads), 4)
        self.assertEqual(len([call for call in calls if 'head-object' in call['args']]), 5)
        self.assertGreater(min(i for i, call in enumerate(calls) if 'head-object' in call['args'] and 'vn/20990101_1/report.json' not in call['args']),
                           max(i for i, call in enumerate(calls) if 'put-object' in call['args']))
        self.assertEqual(calls[-1]['args'], ['workflow', 'run', 'corpus-release.yml', '--ref', 'main',
                                           '-f', 'country=vn', '-f', 'rel=20990102_1'])
        lock_dir = self.path / '.cache/trachuan'
        self.assertEqual(lock_dir.stat().st_mode & 0o777, 0o700)
        self.assertTrue((lock_dir / 'corpus-upload.lock').exists())
        for call in calls:
            if call['name'] == 'aws' and '--version' not in call['args']:
                self.assertIn('trachuan-release-upload', call['args'])
                self.assertIn(self.env['OCI_S3_ENDPOINT'], call['args'])
                self.assertEqual(call['checksum'], 'when_required')
                self.assertIn('addressing_style = path', call['config'])

    def test_dry_run_validates_locally_and_never_calls_clients(self):
        result = self.run_upload('--dry-run')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('s3://trachuan-releases/vn/20990102_1/corpus.dump', result.stdout)
        self.assertIn('corpus-release.yml --ref main', result.stdout)
        self.assertEqual(self.calls(), [])

    def test_existing_prefix_bad_size_and_old_cli_never_dispatch(self):
        for setting, value in [('FAKE_KEY_COUNT', '1'), ('FAKE_BAD_SIZE', 'report.json'),
                               ('FAKE_AWS_VERSION', '1.44.0'), ('FAKE_FAIL_UPLOAD', '1')]:
            with self.subTest(setting=setting):
                self.log.unlink(missing_ok=True)
                shutil.rmtree(self.remote)
                self.remote.mkdir()
                self.env[setting] = value
                result = self.run_upload()
                self.env.pop(setting)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))
                if setting in {'FAKE_KEY_COUNT', 'FAKE_AWS_VERSION'}:
                    self.assertFalse(any('put-object' in call['args'] for call in self.calls()))

    def test_rejected_reports_and_run_state_never_call_clients(self):
        changes = [lambda: self.report.update(quality_pass=False),
                   lambda: self.report.update(dump_sha256='0' * 64),
                   lambda: self.report.update(cap_override='none'),
                   lambda: self.report.update(previous_diff=None),
                   lambda: self.report['previous_diff']['previous'].update(country='us'),
                   lambda: self.report['cohorts']['old_product']['non_regression'][0].update(status='fail'),
                   lambda: self.report['cohorts']['development'].update(non_regression=[]),
                   lambda: self.report['non_regression'][0].update(gating=False),
                   lambda: self.row['run'].update(complete=False)]
        for index, change in enumerate(changes):
            with self.subTest(index=index):
                self.row, self.report = write_fixture(self.build)
                change()
                save_fixture(self.build, self.row, self.report)
                result = self.run_upload()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.calls(), [])

    def test_dispatch_failure_is_reported_without_retrying_upload(self):
        self.env['FAKE_FAIL_DISPATCH'] = '1'
        result = self.run_upload()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(sum(call['name'] == 'gh' for call in self.calls()), 1)

    def test_same_size_replacement_after_upload_prevents_dispatch(self):
        self.env['FAKE_REPLACE_AFTER_UPLOAD'] = 'report.json'
        result = self.run_upload()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))

    def test_previous_report_hash_and_signature_fail_before_any_upload(self):
        for fault in ('report_hash', 'manifest_hash', 'signature', 'missing'):
            with self.subTest(fault=fault):
                self.row, self.report = write_fixture(self.build)
                self.log.unlink(missing_ok=True)
                if fault == 'report_hash':
                    self.report['previous_diff']['previous']['report_sha256'] = '0' * 64
                    save_fixture(self.build, self.row, self.report)
                elif fault == 'manifest_hash':
                    from manifest import canonical
                    path = self.build / 'previous-manifest.json'
                    manifest = json.loads(path.read_bytes())
                    manifest['report']['sha256'] = '0' * 64
                    path.write_bytes(canonical(manifest))
                elif fault == 'signature':
                    self.env['FAKE_BAD_SIGNATURE'] = '1'
                else:
                    (self.build / 'previous-report.json').unlink()
                result = self.run_upload()
                self.env.pop('FAKE_BAD_SIGNATURE', None)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('put-object' in call['args'] or call['name'] == 'gh' for call in self.calls()))

    def test_invalid_report_transitions_fail_upload_and_signing(self):
        from corpus_test_support import digest
        from manifest import canonical

        self.env.update(GITHUB_REF='refs/heads/main', GITHUB_SHA='a' * 40, GITHUB_RUN_NUMBER='10')
        for fault in ('empty development', 'version downgrade'):
            self.row, self.report = write_fixture(self.build)
            self.log.unlink(missing_ok=True)
            if fault == 'empty development':
                self.report['cases'] = [row for row in self.report['cases'] if row['cohort'] != 'development']
                self.report['cohorts']['development'].update(case_count=0, positive_count=0,
                                                             expected_citations=0, metrics=[], non_regression=[])
            else:
                self.report.update(version=1, scorer='agent-eval/4')
                self.report.pop('retrieval_recipe')
                path = self.build / 'previous-report.json'
                previous = json.loads(path.read_bytes())
                previous.update(version=2, scorer='agent-eval/5', retrieval_recipe=self.row['retrieval_recipe'])
                for records in [previous['non_regression'], *(previous['cohorts'][name]['non_regression'] for name in ('old_product', 'development'))]:
                    for record in records:
                        if record['name'] == 'mrr':
                            record.update(previous=1., high_water=1., tolerance=.005, within_tolerance=False, losses=[])
                data = json.dumps(previous).encode()
                path.write_bytes(data)
                self.report['previous_diff']['previous']['report_sha256'] = digest(data)
                path = self.build / 'previous-manifest.json'
                manifest = json.loads(path.read_bytes())
                manifest['report']['sha256'] = digest(data)
                path.write_bytes(canonical(manifest))
            save_fixture(self.build, self.row, self.report)
            commands = [self.upload_command(),
                        ['python3', str(ROOT / 'deploy/github/corpus_manifest.py'), 'create', 'vn', '20990102_1',
                         str(self.build), str(self.build / 'report.json')]]
            for command in commands:
                with self.subTest(fault=fault, executable=command[0]):
                    result = subprocess.run(command, env=self.env, capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0, result.stderr)
                    self.assertFalse((self.build / 'manifest.json').exists())
                    self.assertFalse(any('put-object' in call['args'] or call['name'] == 'gh' for call in self.calls()))

    def test_signing_cli_checks_previous_before_creating_manifest(self):
        self.env.update(GITHUB_REF='refs/heads/main', GITHUB_SHA='a' * 40, GITHUB_RUN_NUMBER='10')
        command = ['python3', str(ROOT / 'deploy/github/corpus_manifest.py'), 'create', 'vn', '20990102_1',
                   str(self.build), str(self.build / 'report.json')]
        result = subprocess.run(command, env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        manifest = self.build / 'manifest.json'
        self.assertTrue(manifest.exists())
        manifest.unlink()
        self.report['previous_diff']['previous']['report_sha256'] = '0' * 64
        save_fixture(self.build, self.row, self.report)
        result = subprocess.run(command, env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(manifest.exists())

    def test_concurrent_uploads_cannot_interleave(self):
        self.assert_uploads_share_lock(self.script)

    def test_concurrent_uploads_from_different_checkouts_share_lock(self):
        other = self.checkout_script('another-repo')
        self.assertNotEqual(other.parent.parent, self.script.parent.parent)
        self.assert_uploads_share_lock(other)

    def assert_uploads_share_lock(self, other_script):
        ready, resume = self.path / 'ready', self.path / 'resume'
        os.mkfifo(ready)
        os.mkfifo(resume)
        descriptor = os.open(ready, os.O_RDONLY | os.O_NONBLOCK)
        self.addCleanup(os.close, descriptor)
        with selectors.DefaultSelector() as selector:
            selector.register(descriptor, selectors.EVENT_READ)
            first_env = self.env | {'FAKE_READY': str(ready), 'FAKE_RESUME': str(resume), 'FAKE_RUN': 'first'}
            with subprocess.Popen(self.upload_command(), env=first_env, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, text=True) as first:
                reached_upload = False
                try:
                    reached_upload = bool(selector.select(timeout=5))
                    self.assertTrue(reached_upload, 'first uploader did not reach upload')
                    os.read(descriptor, 5)
                    result = subprocess.run(self.upload_command(script=other_script), env=self.env | {'FAKE_RUN': 'second'},
                                            capture_output=True, text=True, timeout=5)
                finally:
                    if reached_upload:
                        with resume.open('w') as stream:
                            stream.write('x')
                    elif first.poll() is None:
                        first.terminate()
                stdout, stderr = first.communicate(timeout=5)
                self.assertEqual(first.returncode, 0, stdout + stderr)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(sum(call['name'] == 'gh' for call in self.calls()), 1)
        self.assertFalse(any(call['run'] == 'second' and 'list-objects-v2' in call['args'] for call in self.calls()))

    def test_existing_cache_directory_is_made_private(self):
        lock_dir = self.path / '.cache/trachuan'
        lock_dir.mkdir(parents=True)
        lock_dir.chmod(0o755)
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(lock_dir.stat().st_mode & 0o777, 0o700)

    def test_logs_exact_aws_version(self):
        self.env['FAKE_AWS_VERSION'] = '2.31.0 Python/synthetic synthetic-os'
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('aws-cli/' + self.env['FAKE_AWS_VERSION'], result.stdout)


if __name__ == '__main__':
    unittest.main()
