import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import tempfile
import unittest

import corpus_rules
import corpus_test_support as support
from corpus_evidence import SIDECARS
from corpus_manifest import create
from corpus_test_support import FakeCurl, save_older_shape

ROOT = Path(__file__).resolve().parents[2]
REL = '20990102_1'


def write_fixture(path):
    row, report = support.write_fixture(path)
    save_older_shape(path, row)
    return row, report


def save_fixture(path, row, report):
    support.save_fixture(path, row, report)
    save_older_shape(path, row)


def add_heading_build(path, row, report):
    record = support.add_heading_build(path, row, report)
    save_older_shape(path, row)
    return record


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
    moved = build / 'production-manifest.json'
    production = moved.read_bytes() if moved.exists() else manifest
    signed_path = build / 'signed-manifest.json'
    signed = signed_path.read_bytes() if signed_path.exists() else manifest
    signature = b'synthetic bundle'
    if 'tags' in args:
        print(json.dumps(dict(tags=['vn-production', 'vn-20990101_1'] + (['vn-20990102_1'] if signed_path.exists() else []))))
    elif 'resolve' in args:
        print('sha256:' + ('d' if args[-1].endswith(':vn-production') else 'e' if args[-1].endswith(':vn-20990102_1') else 'c') * 64)
    elif 'manifest' in args:
        selected = production if args[-1].endswith('d' * 64) else signed if args[-1].endswith('e' * 64) else manifest
        print(json.dumps(dict(artifactType='application/vnd.trachuan.corpus-release.v1',
                              layers=[dict(digest='sha256:' + hashlib.sha256(data).hexdigest(), size=len(data),
                                           annotations={'org.opencontainers.image.title': title})
                                      for title, data in [('manifest.json', selected), ('manifest.sigstore.json', signature)]])))
    elif 'blob' in args:
        blobs = {hashlib.sha256(data).hexdigest(): data for data in (manifest, production, signed)}
        sys.stdout.buffer.write(blobs.get(args[-1].rsplit(':', 1)[1], signature))
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
    last = 'build.json' if json.loads((build / 'release.json').read_text())['schema_version'] >= 8 else 'release.json'
    if filename == last and os.getenv('FAKE_REPLACE_AFTER_UPLOAD'):
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
        self.curl = FakeCurl(self.bin, self.log)
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
        shutil.copytree(ROOT / 'deploy/github', repo / 'deploy/github', ignore=shutil.ignore_patterns('__pycache__'))
        rules = repo / 'deploy/github/corpus_rules.py'
        text = rules.read_text()
        cutoff = f"LEGACY_CUTOFF = {{'vn': {corpus_rules.LEGACY_CUTOFF['vn']!r}}}"
        self.assertIn(cutoff, text, 'cutoff literal not found in corpus_rules.py')
        rules.write_text(text.replace(cutoff, f"LEGACY_CUTOFF = {{'vn': {REL!r}}}"))
        (repo / 'sources').mkdir()
        shutil.copyfile(ROOT / 'sources/vn-heading-reviews.yaml', repo / 'sources/vn-heading-reviews.yaml')
        return script

    def repo_tool(self, name):
        return self.script.parents[1] / 'deploy/github' / name

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

    def test_preserves_uploaded_report_in_upload_directory(self):
        external = self.path / 'evaluation report.json'
        external.write_bytes((self.build / 'report.json').read_bytes())
        (self.build / 'report.json').unlink()
        command = self.upload_command()
        command[-1] = str(external)
        result = subprocess.run(command, env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.build / 'report.json').read_bytes(), external.read_bytes())
        self.assertEqual((self.remote / 'report.json').read_bytes(), external.read_bytes())

    def test_dry_run_validates_locally_and_never_calls_clients(self):
        result = self.run_upload('--dry-run')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('s3://trachuan-releases/vn/20990102_1/corpus.dump', result.stdout)
        self.assertIn('corpus-release.yml --ref main', result.stdout)
        self.assertIn('vn-production', result.stdout)
        self.assertIn('https://trachuan.vn/api/v1/public/status', result.stdout)
        self.assertEqual(self.calls(), [])

    def test_upload_checks_served_release_before_any_object(self):
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        status = [index for index, call in enumerate(calls) if call['name'] == 'curl']
        self.assertEqual(len(status), 1)
        self.assertLess(status[0], min(index for index, call in enumerate(calls) if 'put-object' in call['args']))
        self.assertEqual(calls[status[0]]['args'][-1], 'https://trachuan.vn/api/v1/public/status')

    def test_status_request_gets_none_of_the_callers_environment(self):
        self.env.update(HTTPS_PROXY='http://127.0.0.1:9', https_proxy='http://127.0.0.1:9',
                        ALL_PROXY='http://127.0.0.1:9', CURL_CA_BUNDLE=str(self.path / 'ca.pem'))
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        environment = next(call['environment'] for call in self.calls() if call['name'] == 'curl')
        self.assertIn('PATH', environment)
        for name in ('HTTPS_PROXY', 'https_proxy', 'ALL_PROXY', 'CURL_CA_BUNDLE', 'HOME', 'OCI_S3_ENDPOINT'):
            self.assertNotIn(name, environment)

    def test_unsettled_production_never_uploads(self):
        for answer in (dict(release='20990101_2'), dict(code='503'), dict(body=b'not json')):
            with self.subTest(answer=answer):
                self.log.unlink(missing_ok=True)
                self.curl.answer(**answer)
                result = self.run_upload()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('put-object' in call['args'] or call['name'] == 'gh' for call in self.calls()))

    def test_production_other_than_the_compared_release_never_uploads(self):
        from manifest import canonical
        manifest = json.loads((self.build / 'previous-manifest.json').read_bytes())
        for change in (dict(rel='20990101_2', database='corpus_vn_20990101_2',
                            dump=manifest['dump'] | dict(object='vn/20990101_2/corpus.dump')),
                       dict(report=manifest['report'] | dict(sha256='0' * 64))):
            with self.subTest(change=sorted(change)):
                self.log.unlink(missing_ok=True)
                (self.build / 'production-manifest.json').write_bytes(canonical(manifest | change))
                result = self.run_upload()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('not compared with the production release', result.stderr)
                self.assertFalse(any('put-object' in call['args'] or call['name'] in {'gh', 'curl'} for call in self.calls()))

    def test_decision_release_uploads_its_evidence_then_dispatches(self):
        support.save_fixture(self.build, self.row, self.report)
        result = self.run_upload('--dry-run')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.count('Upload and verify size and ETag:'), 7)
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Open findings allowed: review 0, info 0', result.stdout)
        self.assertIn('Findings against 20990101_1: new 0, gone 0', result.stdout)
        calls = self.calls()
        self.assertEqual(sum('put-object' in call['args'] for call in calls), 7)
        for name in SIDECARS:
            self.assertEqual((self.remote / name).read_bytes(), (self.build / name).read_bytes())
        self.assertEqual(calls[-1]['name'], 'gh')

    def test_decision_release_gate_refusals_never_call_clients(self):
        def blocking():
            findings = json.loads((self.build / 'findings.json').read_bytes())
            findings['detectors']['blocker'] = dict(complete=True, severity='blocking', observed=1, not_observable=0, excluded=0)
            findings['findings'] = [dict(key='k', detector='blocker', status='open', carried=True)]
            support.write_evidence(self.build, self.row, self.report, findings=findings)

        def drop_evidence_hashes():
            row = json.loads((self.build / 'release.json').read_bytes())
            for field in ('findings_sha256', 'identity_sha256', 'changes_sha256'):
                row['run'].pop(field)
            (self.build / 'release.json').write_text(json.dumps(row))

        faults = dict(blocking=blocking, older_binary=drop_evidence_hashes,
                      missing=lambda: (self.build / 'changes.jsonl').unlink(),
                      tampered=lambda: (self.build / 'identity.jsonl').write_bytes((self.build / 'identity.jsonl').read_bytes() + b'{}\n'))
        for name, fault in faults.items():
            for flags in ((), ('--dry-run',)):
                with self.subTest(fault=name, flags=flags):
                    support.save_fixture(self.build, self.row, self.report)
                    fault()
                    self.log.unlink(missing_ok=True)
                    result = self.run_upload(*flags)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(self.calls(), [])

    def test_decision_release_predecessor_must_match_the_previous_signed_manifest(self):
        from manifest import canonical
        evidence = dict(findings_sha256='a' * 64, identity_sha256='b' * 64, changes_sha256='c' * 64)
        path = self.build / 'previous-manifest.json'
        previous = json.loads(path.read_bytes())
        path.write_bytes(canonical(previous | dict(evidence=evidence)))
        support.save_fixture(self.build, self.row, self.report)
        result = self.run_upload()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('previous signed manifest', result.stderr)
        self.assertFalse(any('put-object' in call['args'] for call in self.calls()))
        report_bytes = (self.build / 'report.json').read_bytes()
        findings = support.findings_fixture(self.build, self.row, report_bytes, self.report, predecessor='sha256:' + 'a' * 64)
        support.write_evidence(self.build, self.row, self.report, findings=findings, predecessor_identity='sha256:' + 'b' * 64)
        self.log.unlink()
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum('put-object' in call['args'] for call in self.calls()), 7)

    def resign_check(self):
        return subprocess.run(['python3', str(self.repo_tool('corpus_production.py')), 'resign-check', 'vn', REL,
                               str(self.build)], env=self.env, capture_output=True, text=True)

    def signed_manifest(self, **changes):
        from manifest import canonical
        support.save_fixture(self.build, self.row, self.report)
        signed = json.loads(create('c' * 40, 7, 'vn', REL, self.build, self.build / 'report.json'))
        signed.pop('evidence')
        save_older_shape(self.build, self.row)
        (self.build / 'signed-manifest.json').write_bytes(canonical(signed | changes))

    def test_resign_check_compares_the_verified_manifest_with_the_upload_directory(self):
        self.signed_manifest()
        result = self.resign_check()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, 'no signed field would change\n')
        self.signed_manifest(previous='20990101_2')
        result = self.resign_check()
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertEqual(result.stdout, 'previous: signed "20990101_2" rebuilt "20990101_1"\n')
        self.assertFalse(any(call['name'] in {'aws', 'gh', 'curl'} or 'sign-blob' in call['args'] or 'push' in call['args'] or 'tag' in call['args']
                             for call in self.calls()))

    def test_resign_check_refuses_an_unverified_manifest(self):
        self.signed_manifest()
        self.env['FAKE_BAD_SIGNATURE'] = '1'
        result = self.resign_check()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, '')

    def test_schema_eight_uploads_build_record_and_verifies_receipt(self):
        add_heading_build(self.build, self.row, self.report)
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.remote / 'build.json').read_bytes(), (self.build / 'build.json').read_bytes())
        calls = self.calls()
        self.assertEqual(sum('put-object' in call['args'] for call in calls), 5)
        self.assertEqual(sum('head-object' in call['args'] for call in calls), 6)
        self.assertEqual(calls[-1]['name'], 'gh')

    def test_schema_seven_upload_keeps_four_objects_without_build_record(self):
        self.row['schema_version'] = 7
        save_fixture(self.build, self.row, self.report)
        self.assertFalse((self.build / 'build.json').exists())
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum('put-object' in call['args'] for call in self.calls()), 4)
        self.assertEqual({path.name for path in self.remote.iterdir()},
                         {'corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json'})

    def test_schema_eight_build_record_receipt_changes_prevent_dispatch(self):
        add_heading_build(self.build, self.row, self.report)
        for setting in ('FAKE_BAD_SIZE', 'FAKE_REPLACE_AFTER_UPLOAD'):
            with self.subTest(setting=setting):
                self.log.unlink(missing_ok=True)
                shutil.rmtree(self.remote)
                self.remote.mkdir()
                self.env[setting] = 'build.json'
                result = self.run_upload()
                self.env.pop(setting)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))

    def test_schema_seven_and_eight_dry_runs_list_the_required_objects(self):
        self.row['schema_version'] = 7
        save_fixture(self.build, self.row, self.report)
        for version in (7, 8):
            with self.subTest(schema_version=version):
                if version == 8:
                    add_heading_build(self.build, self.row, self.report)
                result = self.run_upload('--dry-run')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual('build.json' in result.stdout, version == 8)
                self.assertEqual(result.stdout.count('Upload and verify size and ETag:'), 5 if version == 8 else 4)
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
                        ['python3', str(self.repo_tool('corpus_manifest.py')), 'create', 'vn', '20990102_1',
                         str(self.build), str(self.build / 'report.json')]]
            for command in commands:
                with self.subTest(fault=fault, executable=command[0]):
                    result = subprocess.run(command, env=self.env, capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0, result.stderr)
                    self.assertFalse((self.build / 'manifest.json').exists())
                    self.assertFalse(any('put-object' in call['args'] or call['name'] == 'gh' for call in self.calls()))

    def test_signing_cli_checks_previous_before_creating_manifest(self):
        self.env.update(GITHUB_REF='refs/heads/main', GITHUB_SHA='a' * 40, GITHUB_RUN_NUMBER='10')
        command = ['python3', str(self.repo_tool('corpus_manifest.py')), 'create', 'vn', '20990102_1',
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
