import importlib
import json
import os
from pathlib import Path
import tempfile
import unittest
import io
import shutil
import subprocess
from contextlib import redirect_stdout
from types import SimpleNamespace
from unittest.mock import patch

from corpus_evidence import LIMITS, SIDECARS
from corpus_upload_test import FAKE
from corpus_test_support import add_heading_build, digest, save_fixture, save_older_shape, write_fixture


class CorpusStorageTest(unittest.TestCase):
    def setUp(self):
        self.storage = importlib.import_module('corpus_storage')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.remote = self.path / 'remote'
        self.remote.mkdir()
        self.row, self.report = write_fixture(self.remote)
        self.row['schema_version'] = 7
        save_fixture(self.remote, self.row, self.report)
        executable = self.path / 'aws'
        executable.write_text(FAKE)
        executable.chmod(0o755)
        self.log = self.path / 'calls.jsonl'
        self.env = dict(PATH=str(self.path) + os.pathsep + os.environ['PATH'],
                        FAKE_BUILD=str(self.remote), FAKE_LOG=str(self.log),
                        OCI_S3_ENDPOINT='https://synthetic.example', PYTHONDONTWRITEBYTECODE='1',
                        FAKE_AWS_VERSION='2.40.0')

    def report_sha256(self):
        return digest((self.remote / 'report.json').read_bytes())

    def gets(self):
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return [call['args'][call['args'].index('--key') + 1] for call in calls if 'get-object' in call['args']]

    def test_report_is_the_first_object_fetched(self):
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'first')
        self.assertEqual(self.gets()[0], 'vn/20990102_1/report.json')

    def test_report_with_another_hash_stops_before_any_other_object(self):
        dispatched = self.report_sha256()
        swapped = (self.remote / 'report.json').read_bytes() + b' '
        (self.remote / 'report.json').write_bytes(swapped)
        output = self.path / 'swapped'
        for value in (dispatched, '0' * 64):
            with self.subTest(value=value), patch.dict(os.environ, self.env, clear=True), \
                    self.assertRaisesRegex(ValueError, 'dispatched SHA-256'):
                self.storage.download('vn', '20990102_1', value, output)
            self.assertEqual(self.gets(), ['vn/20990102_1/report.json'])
            self.assertFalse((output / 'corpus.dump').exists())
            self.log.unlink()
            shutil.rmtree(output)

    def test_malformed_report_hash_never_calls_clients(self):
        for value in ('', 'short', self.report_sha256().upper(), 'g' * 64, self.report_sha256() + '\n', None):
            with self.subTest(value=value), patch.dict(os.environ, self.env, clear=True), \
                    self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', value, self.path / 'malformed')
            self.assertFalse(self.log.exists())
            self.assertFalse((self.path / 'malformed').exists())

    def test_downloads_all_seven_objects_of_a_decision_release_with_path_style_and_size_checks(self):
        output = self.path / 'download'
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', self.report_sha256(), output)
        for name in ('corpus.dump', 'MANIFEST.sha256', 'release.json', 'report.json') + SIDECARS:
            self.assertEqual((output / name).read_bytes(), (self.remote / name).read_bytes())
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertEqual(sum('get-object' in call['args'] for call in calls), 7)
        self.assertGreater(min(i for i, call in enumerate(calls) if 'get-object' in call['args']),
                           max(i for i, call in enumerate(calls) if 'head-object' in call['args']))
        for call in calls:
            if 'get-object' in call['args']:
                args = call['args']
                name = args[args.index('--key') + 1].rsplit('/', 1)[1]
                expected = (self.remote / name).stat().st_size
                self.assertEqual(args[args.index('--range') + 1], f'bytes=0-{expected - 1}')
                self.assertIn('--if-match', args)
                self.assertEqual(args[args.index('--if-match') + 1], '"' + digest((self.remote / name).read_bytes()) + '"')
                self.assertEqual(args[args.index('--output') + 1], 'json')
        for call in calls[1:]:
            self.assertEqual(call['checksum'], 'when_required')
            self.assertIn('addressing_style = path', call['config'])

    def test_size_mismatch_and_invalid_identity_stop_download(self):
        with patch.dict(os.environ, self.env | {'FAKE_BAD_SIZE': 'corpus.dump'}, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'download')
        self.log.unlink()
        with self.assertRaises(ValueError):
            self.storage.download('../', '20990102_1', self.report_sha256(), self.path / 'unused')
        self.assertFalse(self.log.exists())

    def test_older_shape_downloads_four_objects_and_never_its_stray_evidence(self):
        save_older_shape(self.remote, self.row)
        output = self.path / 'older'
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', self.report_sha256(), output)
        self.assertEqual(sorted(path.name for path in output.iterdir()), sorted(['corpus.dump', 'MANIFEST.sha256', 'release.json', 'report.json']))
        for name in SIDECARS:
            (self.remote / name).unlink()
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'older-without-evidence')

    def test_decision_release_missing_evidence_refuses_before_dump_download(self):
        for name in SIDECARS:
            with self.subTest(name=name):
                data = (self.remote / name).read_bytes()
                (self.remote / name).unlink()
                self.log.unlink(missing_ok=True)
                with patch.dict(os.environ, self.env, clear=True), self.assertRaisesRegex(ValueError, 'evidence files'):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / ('missing-' + name))
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                self.assertFalse(any('get-object' in call['args'] and 'vn/20990102_1/corpus.dump' in call['args'] for call in calls))
                (self.remote / name).write_bytes(data)

    def test_evidence_limits_are_sixteen_mib_and_one_gib(self):
        self.assertEqual(LIMITS, {'findings.json': 16 * 1024**2, 'identity.jsonl': 1024**3, 'changes.jsonl': 1024**3})

    def test_rejects_every_object_limit_before_downloading(self):
        limits = {'corpus.dump': 16 * 1024**3, 'report.json': 16 * 1024**2,
                  'release.json': 16 * 1024**2, 'MANIFEST.sha256': 1024**2} | LIMITS
        for name, limit in limits.items():
            with self.subTest(name=name), patch.dict(os.environ, self.env | {
                    'FAKE_HEAD_SIZES': json.dumps({name: limit + 1})}, clear=True):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / name)
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                self.assertFalse(any('get-object' in call['args'] or 'cp' in call['args'] for call in calls))
                self.log.unlink()

    def test_checks_aggregate_against_free_space_with_two_gib_reserved(self):
        total = sum((self.remote / name).stat().st_size for name in ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json') + SIDECARS)
        reserve = 2 * 1024**3
        for free in (reserve - 1, reserve + total - 1, reserve + total):
            with self.subTest(free=free), patch.dict(os.environ, self.env, clear=True), \
                    patch('shutil.disk_usage', return_value=SimpleNamespace(free=free)):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / str(free))
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                self.assertFalse(any('get-object' in call['args'] or 'cp' in call['args'] for call in calls))
                self.log.unlink()

    def test_growth_after_head_refuses_even_when_the_prefix_matches(self):
        output = self.path / 'bounded'
        with patch.dict(os.environ, self.env | {'FAKE_GROW_AFTER_HEAD': 'corpus.dump'}, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), output)
        self.assertEqual((output / 'corpus.dump').read_bytes(), (self.remote / 'corpus.dump').read_bytes())
        with patch.dict(os.environ, self.env | {'FAKE_GROW_AFTER_HEAD': 'corpus.dump',
                                              'FAKE_IGNORE_RANGE': 'corpus.dump'}, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'bad-range')

    def test_response_identity_and_total_are_required_when_if_match_is_ignored(self):
        name = 'corpus.dump'
        size = (self.remote / name).stat().st_size
        original_etag = '"' + digest((self.remote / name).read_bytes()) + '"'
        mutations = [
            ({'FAKE_GROW_AFTER_HEAD': name}, {'ETag': original_etag}),
            ({}, {'ETag': '"replacement-with-the-same-prefix"'}),
            ({}, {'ETag': None}),
            ({}, {'ContentRange': None}),
            ({}, {'ContentRange': f'bytes 0-{size - 1}/*'}),
            ({}, {'ContentRange': f'bytes 1-{size}/{size}'}),
            ({}, {'ContentRange': f'bytes 0-{size - 2}/{size}'}),
        ]
        for index, (extra_env, metadata) in enumerate(mutations):
            output = self.path / f'replaced-{index}'
            env = self.env | extra_env | {'FAKE_GET_METADATA': json.dumps({name: metadata})}
            with self.subTest(metadata=metadata), patch.dict(os.environ, env, clear=True):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), output)
                self.assertEqual((output / name).read_bytes(), (self.remote / name).read_bytes())

    def test_missing_head_etag_refuses_before_downloading(self):
        env = self.env | {'FAKE_HEAD_ETAGS': json.dumps({'corpus.dump': None})}
        with patch.dict(os.environ, env, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'missing-etag')
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertFalse(any('get-object' in call['args'] for call in calls))

    def test_logs_exact_aws_version(self):
        version = '2.40.0 Python/synthetic synthetic-os'
        with patch.dict(os.environ, self.env | {'FAKE_AWS_VERSION': version}, clear=True), \
                redirect_stdout(io.StringIO()) as output:
            self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'logged')
        self.assertIn('aws-cli/' + version, output.getvalue())

    def test_rejects_other_aws_versions_before_download(self):
        for version in ('2.39.0', '2.40.01', '2.40.0rc1', '3.0.0'):
            with self.subTest(version=version), patch.dict(os.environ, self.env | {
                    'FAKE_AWS_VERSION': version}, clear=True):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / version)
                self.assertFalse((self.path / version).exists())

    def test_schema_eight_downloads_all_eight_objects_after_all_heads(self):
        add_heading_build(self.remote, self.row, self.report)
        output = self.path / 'schema-eight'
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', self.report_sha256(), output)
        names = ('corpus.dump', 'MANIFEST.sha256', 'release.json', 'report.json', 'build.json') + SIDECARS
        for name in names:
            self.assertEqual((output / name).read_bytes(), (self.remote / name).read_bytes())
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertEqual(sum('get-object' in call['args'] for call in calls), 8)
        self.assertLess(max(i for i, call in enumerate(calls) if 'head-object' in call['args']),
                        min(i for i, call in enumerate(calls) if 'get-object' in call['args']))
        for call in calls:
            args = call['args']
            if 'get-object' not in args:
                continue
            name = args[args.index('--key') + 1].rsplit('/', 1)[1]
            size = (self.remote / name).stat().st_size
            self.assertEqual(args[args.index('--range') + 1], f'bytes=0-{size - 1}')
            self.assertEqual(args[args.index('--if-match') + 1], '"' + digest((self.remote / name).read_bytes()) + '"')

    def test_schema_eight_build_record_limit_and_identity_checks(self):
        add_heading_build(self.remote, self.row, self.report)
        faults = [{'FAKE_HEAD_SIZES': json.dumps({'build.json': 16 * 1024**2 + 1})},
                  {'FAKE_BAD_SIZE': 'build.json'},
                  {'FAKE_HEAD_ETAGS': json.dumps({'build.json': None})},
                  {'FAKE_GROW_AFTER_HEAD': 'build.json'},
                  {'FAKE_GET_METADATA': json.dumps({'build.json': {'ETag': '"different"'}})},
                  {'FAKE_GET_METADATA': json.dumps({'build.json': {'ContentRange': None}})}]
        for index, fault in enumerate(faults):
            with self.subTest(fault=fault), patch.dict(os.environ, self.env | fault, clear=True):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / f'fault-{index}')

    def test_schema_eight_missing_build_record_refuses_before_dump_download(self):
        add_heading_build(self.remote, self.row, self.report)
        (self.remote / 'build.json').unlink()
        with patch.dict(os.environ, self.env, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'missing-build')
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertFalse(any('get-object' in call['args'] and 'vn/20990102_1/corpus.dump' in call['args'] for call in calls))

    def test_schema_eight_counts_build_record_in_free_space_check(self):
        add_heading_build(self.remote, self.row, self.report)
        total = sum((self.remote / name).stat().st_size for name in
                    ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json', 'build.json') + SIDECARS)
        with patch.dict(os.environ, self.env, clear=True), \
                patch('shutil.disk_usage', return_value=SimpleNamespace(free=2 * 1024**3 + total)):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.report_sha256(), self.path / 'space-boundary')
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertFalse(any('get-object' in call['args'] for call in calls))

    def test_workflow_download_step_transports_build_record(self):
        import yaml
        add_heading_build(self.remote, self.row, self.report)
        root = Path(__file__).resolve().parents[2]
        workflow = yaml.safe_load((root / '.github/workflows/corpus-release.yml').read_text())
        step = next(step for step in workflow['jobs']['release']['steps'] if step.get('name', '').startswith('Download corpus objects'))
        output = self.path / 'workflow-download'
        env = self.env | dict(COUNTRY='vn', REL='20990102_1', REPORT_SHA256=self.report_sha256(), RELEASE_DIR=str(output))
        forged = subprocess.run(['bash', '-c', step['run']], env=env | dict(REPORT_SHA256='0' * 64, RELEASE_DIR=str(self.path / 'forged')),
                                cwd=root, capture_output=True, text=True)
        self.assertNotEqual(forged.returncode, 0)
        self.assertEqual(self.gets(), ['vn/20990102_1/report.json'])
        self.assertFalse((self.path / 'forged/corpus.dump').exists())
        result = subprocess.run(['bash', '-c', step['run']], env=env, cwd=root, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((output / 'build.json').read_bytes(), (self.remote / 'build.json').read_bytes())


if __name__ == '__main__':
    unittest.main()
