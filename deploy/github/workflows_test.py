from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class WorkflowsTest(unittest.TestCase):
    def test_release_gate_and_registry_attestations(self):
        text = (ROOT / '.github/workflows/images.yml').read_text()
        self.assertIn('\n  release:\n', text)
        release = text.split('\n  release:\n', 1)[1]
        for line in ('    needs: images', "    if: github.event_name == 'push'",
                     '    environment: production', '    timeout-minutes: 75',
                     '          fetch-depth: 0', '          persist-credentials: false'):
            self.assertIn(line, release)
        self.assertEqual(text.count('push-to-registry: true'), 4)

    def test_rollback_gate_and_no_verification_bypasses(self):
        path = ROOT / '.github/workflows/rollback.yml'
        self.assertTrue(path.exists(), 'rollback workflow missing')
        text = path.read_text()
        self.assertIn("if: github.ref == 'refs/heads/main'", text)
        self.assertIn('environment: production', text)
        for path in [*(ROOT / '.github/workflows').glob('*.yml'),
                     *(ROOT / 'deploy/github').glob('*.py'),
                     *(ROOT / 'deploy/github').glob('*.sh')]:
            if path.name.endswith('_test.py'):
                continue
            for flag in ('--insecure-ignore-tlog', '--insecure-ignore-sct', '--allow-insecure-registry'):
                self.assertNotIn(flag, path.read_text(), str(path))

    def test_actions_are_pinned(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            for action in re.findall(r'uses: (\S+)', path.read_text()):
                self.assertRegex(action, r'^[\w./-]+@[0-9a-f]{40}$')


if __name__ == '__main__':
    unittest.main()
