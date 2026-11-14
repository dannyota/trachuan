import importlib
from pathlib import Path
import unittest
from unittest.mock import patch


class SigningTest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(Path(__file__).with_name('signing.py').exists(), 'signing helper missing')
        self.s = importlib.import_module('signing')

    def test_signs_then_verifies_exact_release_identity_and_commit(self):
        with patch('signing.subprocess.run') as run:
            self.s.sign('manifest.json', 'manifest.sigstore.json', 'images.yml', 'refs/tags/v1.2.3', 'a' * 40)
        self.assertEqual(run.call_args_list[0].args[0],
                         ['cosign', 'sign-blob', '--yes', '--bundle', 'manifest.sigstore.json', 'manifest.json'])
        command = run.call_args_list[1].args[0]
        self.assertEqual(command[command.index('--certificate-identity') + 1],
                         'https://github.com/dannyota/trachuan/.github/workflows/images.yml@refs/tags/v1.2.3')
        self.assertEqual(command[command.index('--certificate-github-workflow-sha') + 1], 'a' * 40)
        self.assertEqual(command[command.index('--certificate-oidc-issuer') + 1],
                         'https://token.actions.githubusercontent.com')

    def test_rollback_uses_main_identity(self):
        with patch('signing.subprocess.run') as run:
            self.s.sign('manifest.json', 'manifest.sigstore.json', 'rollback.yml', 'refs/heads/main', 'b' * 40)
        self.assertIn('https://github.com/dannyota/trachuan/.github/workflows/rollback.yml@refs/heads/main',
                      run.call_args.args[0])

    def test_discovered_release_binds_ref_and_commit_with_anchored_identity(self):
        with patch('signing.subprocess.run') as run:
            self.s.verify('manifest.json', 'sig.json', 'images.yml', 'refs/tags/v1.2.3', 'c' * 40, release=True)
        command = run.call_args.args[0]
        self.assertIn('--certificate-identity-regexp', command)
        self.assertTrue(command[command.index('--certificate-identity-regexp') + 1].startswith('^'))
        self.assertTrue(command[command.index('--certificate-identity-regexp') + 1].endswith('$'))
        self.assertEqual(command[command.index('--certificate-github-workflow-ref') + 1], 'refs/tags/v1.2.3')
        self.assertEqual(command[command.index('--certificate-github-workflow-sha') + 1], 'c' * 40)


if __name__ == '__main__':
    unittest.main()
