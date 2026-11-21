from repositories_test_support import enable_dual_policy as setUpModule

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import yaml

from frameworks_test_support import COMMIT, REL, SENTINEL, calls, install_fakes, object_bytes, report_hash, state

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/frameworks-release.yml'
LAW = ROOT / '.github/workflows/corpus-release.yml'
sys.path.append(str(ROOT / 'deploy/host'))


class FrameworksWorkflowTest(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.workflow = yaml.safe_load(self.text)
        self.job = self.workflow['jobs']['release']
        self.steps = self.job['steps']

    def step(self, prefix):
        return next(step for step in self.steps if step.get('name', '').startswith(prefix))

    def test_dispatch_environment_permissions_and_queue(self):
        triggers = self.workflow[True]
        self.assertEqual(list(triggers), ['workflow_dispatch'])
        inputs = triggers['workflow_dispatch']['inputs']
        self.assertEqual(list(inputs), ['rel', 'report_sha256'])
        self.assertTrue(all(item['required'] is True and item['type'] == 'string' for item in inputs.values()))
        self.assertEqual(self.workflow['permissions'], {'contents': 'read'})
        self.assertEqual(self.job['if'], "github.ref == 'refs/heads/main'")
        self.assertEqual(self.job['environment'], 'frameworks-release')
        self.assertEqual(self.job['permissions'], {'contents': 'read', 'packages': 'write', 'id-token': 'write'})
        self.assertEqual(self.job['concurrency'], {'group': 'frameworks-release-production',
                                                   'cancel-in-progress': False, 'queue': 'max'})
        self.assertEqual(self.job['runs-on'], 'ubuntu-24.04-arm')

    def test_identity_is_validated_before_anything_else(self):
        first = self.steps[0]
        self.assertIn('[[ $GITHUB_REF == refs/heads/main ]]', first['run'])
        self.assertIn('[[ $REL =~ ^[0-9]{8}(_[0-9]{1,2})?$ ]]', first['run'])
        self.assertIn('[[ $REPORT_SHA256 =~ ^[0-9a-f]{64}$ ]]', first['run'])
        self.assertEqual(self.job['env']['REL'], '${{ inputs.rel }}')
        self.assertEqual(self.job['env']['REPORT_SHA256'], '${{ inputs.report_sha256 }}')
        self.assertNotIn('${{ inputs', ''.join(step.get('run', '') for step in self.steps))

    def test_download_and_signing_receive_the_dispatched_report_hash(self):
        self.assertEqual(self.step('Download frameworks objects')['run'],
                         'python3 deploy/github/frameworks_storage.py "$REL" "$REPORT_SHA256" "$RELEASE_DIR/objects"')
        self.assertIn('python3 deploy/github/frameworks_publish.py "$REL" "$REPORT_SHA256" "$RELEASE_DIR"',
                      self.step('Validate, sign')['run'])

    def test_law_workflow_keeps_its_inputs(self):
        inputs = yaml.safe_load(LAW.read_text())[True]['workflow_dispatch']['inputs']
        self.assertEqual(list(inputs), ['country', 'rel', 'report_sha256', 'release_sha256'])

    def test_tools_and_checkout_are_pinned_like_the_law_job(self):
        law = yaml.safe_load(LAW.read_text())['jobs']['release']['steps']
        checkout = next(step for step in self.steps if 'uses' in step)
        law_checkout = next(step for step in law if 'uses' in step)
        self.assertEqual(law_checkout['with'].pop('fetch-depth'), 0)
        self.assertEqual(checkout, law_checkout)
        self.assertEqual(self.step('Install AWS CLI')['run'], next(s for s in law if s.get('name') == 'Install AWS CLI')['run'])
        self.assertEqual([step['uses'] for step in self.steps if 'uses' in step], [checkout['uses']])
        self.assertEqual(self.step('Install signing')['run'], 'bash deploy/github/install-tools.sh')

    def test_only_the_download_step_holds_the_read_key(self):
        secrets = [step for step in self.steps if 'FRAMEWORKS_READ_KEY' in json.dumps(step)]
        self.assertEqual(secrets, [self.step('Download frameworks objects')])
        env = secrets[0]['env']
        self.assertEqual(env['AWS_ACCESS_KEY_ID'], '${{ secrets.FRAMEWORKS_READ_KEY_ID }}')
        self.assertEqual(env['AWS_SECRET_ACCESS_KEY'], '${{ secrets.FRAMEWORKS_READ_KEY_SECRET }}')
        self.assertEqual(env['OCI_S3_ENDPOINT'], '${{ vars.OCI_S3_ENDPOINT }}')
        self.assertNotIn('RELEASES_READ_KEY', self.text)
        for step in self.steps:
            self.assertNotIn('secrets.', step.get('run', ''))

    def test_temporary_files_stay_in_the_release_directory_and_are_always_deleted(self):
        first = self.steps[0]['run']
        self.assertIn('mkdir -m 700 "$RUNNER_TEMP/trachuan-frameworks-release"', first)
        self.assertIn('TMPDIR=$RUNNER_TEMP/trachuan-frameworks-release/tmp', first)
        last = self.steps[-1]
        self.assertEqual(last['if'], 'always()')
        self.assertEqual(last['run'], 'rm -rf -- "$RUNNER_TEMP/trachuan-frameworks-release"')
        self.assertEqual([step for step in self.steps if 'if' in step], [last])

    def test_no_artifact_cache_or_content_output(self):
        self.assertNotRegex(self.text, r'upload-artifact|download-artifact|actions/cache|setup-python|cache:|GITHUB_STEP_SUMMARY|GITHUB_OUTPUT')
        self.assertNotIn('set -x', self.text)
        self.assertNotIn('continue-on-error', self.text)
        for step in self.steps:
            run = step.get('run', '')
            self.assertNotRegex(run, r'\b(cat|jq|head|tail)\b')
            self.assertNotRegex(run, r'(echo|printenv|env\s*$).*AWS_')

    def test_verification_is_never_bypassed(self):
        for flag in ('--insecure-ignore-tlog', '--insecure-ignore-sct', '--allow-insecure-registry'):
            self.assertNotIn(flag, self.text)

    def test_steps_download_sign_and_clean_up_through_the_real_commands(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary)
            install_fakes(path / 'bin')
            runner = path / 'runner'
            runner.mkdir()
            remote = path / 'remote'
            registry = path / 'registry'
            registry.mkdir()
            log = path / 'calls.jsonl'
            stored = remote / 'trachuan-frameworks-releases/frameworks' / REL
            stored.mkdir(parents=True)
            value = state()
            for name, data in object_bytes(value).items():
                (stored / name).write_bytes(data)
            github_env = path / 'github-env'
            env = dict(PATH=str(path / 'bin') + os.pathsep + os.environ['PATH'], HOME=str(path), RUNNER_TEMP=str(runner),
                       GITHUB_REF='refs/heads/main', GITHUB_SHA=COMMIT, GITHUB_RUN_NUMBER='5', GITHUB_ENV=str(github_env),
                       GITHUB_REPOSITORY='dntmakers/trachuan',
                       REL=REL, REPORT_SHA256=report_hash(value), PYTHONDONTWRITEBYTECODE='1', FAKE_LOG=str(log), FAKE_REMOTE=str(remote),
                       FAKE_REGISTRY=str(registry), OCI_S3_ENDPOINT='https://synthetic.example',
                       GITHUB_TOKEN='synthetic', ACTOR='synthetic')

            def run(step):
                result = subprocess.run(['bash', '-e', '-c', step['run']], env=env, cwd=ROOT, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotIn(SENTINEL, result.stdout + result.stderr)
                self.assertNotIn('SIGNATURE-ON-STDOUT', result.stdout + result.stderr)

            run(self.steps[0])
            exported = dict(line.split('=', 1) for line in github_env.read_text().splitlines())
            self.assertEqual(set(exported), {'RELEASE_DIR', 'TMPDIR'})
            env.update(exported)
            forged = subprocess.run(['bash', '-e', '-c', self.step('Download frameworks objects')['run']],
                                    env=env | dict(REPORT_SHA256='0' * 64, RELEASE_DIR=str(path / 'forged')), cwd=ROOT,
                                    capture_output=True, text=True)
            self.assertNotEqual(forged.returncode, 0)
            self.assertFalse((path / 'forged/objects/frameworks.dump').exists())
            run(self.step('Download frameworks objects'))
            run(self.step('Validate, sign'))
            data = (registry / f'frameworks-{REL}/manifest.json').read_bytes()
            self.assertEqual((registry / 'frameworks-production/manifest.json').read_bytes(), data)
            from updater import deploy_frameworks_verify
            from frameworks_manifest_test import RULES
            self.assertEqual(deploy_frameworks_verify.manifest(data, RULES)['sequence'], 5)
            self.assertEqual({call['tmpdir'] for call in calls(log)}, {exported['TMPDIR']})
            run(self.steps[-1])
            self.assertFalse(Path(exported['RELEASE_DIR']).exists())
            self.assertEqual(list(runner.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
