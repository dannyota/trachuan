from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


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


if __name__ == '__main__':
    unittest.main()
