import json
import os
from pathlib import Path
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
args = sys.argv[1:]
name = Path(sys.argv[0]).name
with open(os.environ['FAKE_LOG'], 'a') as stream:
    stream.write(json.dumps(dict(name=name, args=args, checksum=os.getenv('AWS_REQUEST_CHECKSUM_CALCULATION'),
                                config=Path(os.environ['AWS_CONFIG_FILE']).read_text() if 'AWS_CONFIG_FILE' in os.environ else '')) + '\\n')
if '--version' in args:
    print('aws-cli/' + os.getenv('FAKE_AWS_VERSION', '2.31.0'))
elif 'list-objects-v2' in args:
    print(os.getenv('FAKE_KEY_COUNT', '0'))
elif 'head-object' in args:
    filename = args[args.index('--key') + 1].rsplit('/', 1)[1]
    size = (Path(os.environ['FAKE_BUILD']) / filename).stat().st_size
    print(size + (1 if os.getenv('FAKE_BAD_SIZE') == filename else 0))
elif name == 'aws' and 'cp' in args:
    index = args.index('cp')
    if args[index + 1].startswith('s3://'):
        source = Path(os.environ['FAKE_BUILD']) / args[index + 1].rsplit('/', 1)[1]
        shutil.copyfile(source, args[index + 2])
    if os.getenv('FAKE_FAIL_UPLOAD'):
        sys.exit(1)
elif name == 'gh' and os.getenv('FAKE_FAIL_DISPATCH'):
    sys.exit(1)
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
        for name in ('aws', 'gh'):
            executable = self.bin / name
            executable.write_text(FAKE)
            executable.chmod(0o755)
        self.log = self.path / 'calls.jsonl'
        self.env = dict(PATH=str(self.bin) + os.pathsep + os.environ['PATH'], HOME=str(self.path),
                        OCI_S3_ENDPOINT='https://synthetic.compat.objectstorage.ap-singapore-1.oraclecloud.com',
                        FAKE_LOG=str(self.log), FAKE_BUILD=str(self.build), PYTHONDONTWRITEBYTECODE='1')

    def run_upload(self, *flags):
        return subprocess.run(['bash', str(ROOT / 'scripts/corpus-upload.sh'), *flags,
                               'vn', '20990102_1', str(self.build), str(self.build / 'report.json')],
                              env=self.env, capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_uploads_four_objects_verifies_sizes_then_dispatches_main(self):
        result = self.run_upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        uploads = [call for call in calls if 'cp' in call['args']]
        self.assertEqual(len(uploads), 4)
        self.assertEqual(len([call for call in calls if 'head-object' in call['args']]), 4)
        self.assertEqual(calls[-1]['args'], ['workflow', 'run', 'corpus-release.yml', '--ref', 'main',
                                           '-f', 'country=vn', '-f', 'rel=20990102_1'])
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
                self.env[setting] = value
                result = self.run_upload()
                self.env.pop(setting)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))
                if setting in {'FAKE_KEY_COUNT', 'FAKE_AWS_VERSION'}:
                    self.assertFalse(any('cp' in call['args'] for call in self.calls()))

    def test_rejected_reports_and_run_state_never_call_clients(self):
        changes = [lambda: self.report.update(quality_pass=False),
                   lambda: self.report.update(dump_sha256='0' * 64),
                   lambda: self.report.update(cap_override='none'),
                   lambda: self.report.update(previous_diff=None),
                   lambda: self.report['previous_diff']['previous'].update(country='us'),
                   lambda: self.report['cohorts']['old_product']['non_regression'][0].update(status='fail'),
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


if __name__ == '__main__':
    unittest.main()
