import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/corpus-release.yml'
HASH = 'a' * 64


class CorpusWorkflowTest(unittest.TestCase):
    def test_main_environment_permissions_and_queue(self):
        text = (ROOT / '.github/workflows/corpus-release.yml').read_text()
        for required in ('workflow_dispatch:', "if: github.ref == 'refs/heads/main'",
                         'environment: corpus-release', 'group: corpus-release-production',
                         'cancel-in-progress: false', 'queue: max', 'contents: read',
                         'packages: write', 'id-token: write',
                         'persist-credentials: false', '[[ $GITHUB_REF == refs/heads/main ]]',
                         '^[a-z]{2}$', '^[0-9]{8}_[0-9]+$',
                         'bash deploy/github/install-tools.sh',
                         'python3 deploy/github/corpus_storage.py',
                         'python3 deploy/github/corpus_manifest.py create',
                         'python3 deploy/github/corpus_publish.py'):
            self.assertIn(required, text)
        self.assertIn('.github/workflows/corpus-release.yml:', (ROOT / '.github/actionlint.yaml').read_text())

    def test_checkout_holds_the_full_main_history_for_fixture_lookup(self):
        text = (ROOT / '.github/workflows/corpus-release.yml').read_text()
        checkout = text[text.index('uses: actions/checkout@'):text.index('Install AWS CLI')]
        self.assertIn('          fetch-depth: 0\n', checkout)
        self.assertIn('          persist-credentials: false\n', checkout)

    def test_installs_verified_aws_before_credentials(self):
        text = (ROOT / '.github/workflows/corpus-release.yml').read_text()
        install = text.index('Install AWS CLI')
        download = text.index('Download corpus objects')
        self.assertLess(install, download)
        step = text[install:download]
        for value in ('awscli-exe-linux-aarch64-2.40.0.zip',
                      'd293743e9e79dc69dc79dcbc23e26d1258193f0dd1253ba0c09ccf56b6123c6e',
                      'sha256sum --check --strict', '$RUNNER_TEMP', '$GITHUB_PATH', 'aws-cli/2.40.0'):
            self.assertIn(value, step)
        self.assertNotIn('sudo', step)
        self.assertNotIn('--update', step)

    def test_secrets_are_environment_only_and_verification_has_no_skip_flags(self):
        text = (ROOT / '.github/workflows/corpus-release.yml').read_text()
        self.assertIn('AWS_ACCESS_KEY_ID: ${{ secrets.RELEASES_READ_KEY_ID }}', text)
        self.assertIn('AWS_SECRET_ACCESS_KEY: ${{ secrets.RELEASES_READ_KEY_SECRET }}', text)
        self.assertIn('OCI_S3_ENDPOINT: ${{ vars.OCI_S3_ENDPOINT }}', text)
        for run in re.findall(r'run: \|\n((?:          .*\n|\n)+)', text):
            self.assertNotIn('secrets.', run)
            self.assertNotRegex(run, r'(echo|printenv|set -x|env\s*$).*AWS_')
        for path in (ROOT / 'deploy/github').glob('*.py'):
            if path.name.endswith('_test.py'):
                continue
            for flag in ('--insecure-ignore-tlog', '--insecure-ignore-sct', '--allow-insecure-registry'):
                self.assertNotIn(flag, path.read_text())


class CorpusDispatchInputTest(unittest.TestCase):
    def setUp(self):
        self.workflow = yaml.safe_load(WORKFLOW.read_text())
        self.job = self.workflow['jobs']['release']
        self.steps = self.job['steps']

    def step(self, prefix):
        return next(step for step in self.steps if step.get('name', '').startswith(prefix))

    def test_dispatch_takes_three_required_string_inputs(self):
        inputs = self.workflow[True]['workflow_dispatch']['inputs']
        self.assertEqual(list(inputs), ['country', 'rel', 'report_sha256'])
        self.assertTrue(all(item['required'] is True and item['type'] == 'string' for item in inputs.values()))

    def test_report_hash_reaches_commands_only_through_the_environment(self):
        self.assertEqual(self.job['env']['REPORT_SHA256'], '${{ inputs.report_sha256 }}')
        self.assertIn('[[ $REPORT_SHA256 =~ ^[0-9a-f]{64}$ ]]', self.steps[0]['run'])
        self.assertNotIn('${{ inputs', ''.join(step.get('run', '') for step in self.steps))
        self.assertEqual(self.step('Download corpus objects')['run'],
                         'python3 deploy/github/corpus_storage.py "$COUNTRY" "$REL" "$REPORT_SHA256" "$RELEASE_DIR"')
        self.assertIn('python3 deploy/github/corpus_publish.py "$REPORT_SHA256" "$RELEASE_DIR"',
                      self.step('Validate, sign')['run'])

    def first_step(self, **values):
        with tempfile.TemporaryDirectory() as temporary:
            github_env = Path(temporary) / 'github-env'
            env = dict(PATH=os.environ['PATH'], GITHUB_REF='refs/heads/main', GITHUB_ENV=str(github_env),
                       RUNNER_TEMP=temporary, COUNTRY='vn', REL='20990102_1', REPORT_SHA256=HASH) | values
            env = {key: value for key, value in env.items() if value is not None}
            result = subprocess.run(['bash', '-e', '-c', self.steps[0]['run']], env=env, capture_output=True, text=True)
            return result.returncode, github_env.exists()

    def test_first_step_refuses_a_missing_or_malformed_report_hash(self):
        self.assertEqual(self.first_step(), (0, True))
        for value in (None, '', 'a' * 63, 'a' * 65, 'A' * 64, 'a' * 63 + 'F', 'g' * 64, HASH + '\n',
                      ' ' + 'a' * 63, 'sha256:' + HASH, 'a' * 32 + ' ' + 'a' * 31):
            with self.subTest(value=value):
                self.assertEqual(self.first_step(REPORT_SHA256=value), (1, False))
        for values in (dict(COUNTRY='VN'), dict(REL='20990102')):
            with self.subTest(values=values):
                self.assertEqual(self.first_step(**values), (1, False))


if __name__ == '__main__':
    unittest.main()
