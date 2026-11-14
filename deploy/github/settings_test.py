import importlib
import io
import json
from pathlib import Path
import unittest
from contextlib import redirect_stdout
from unittest.mock import patch


class SettingsTest(unittest.TestCase):
    def setUp(self):
        path = Path(__file__).with_name('settings.json')
        self.assertTrue(path.exists(), 'repository settings missing')
        self.config = json.loads(path.read_text())

    def test_rulesets_enforce_separate_create_and_fixed_policies(self):
        rulesets = {r['name']: r for r in self.config['rulesets']}
        self.assertEqual(set(rulesets), {'main', 'release-tags-fixed', 'release-tags-create'})
        for name, target, refs, rules in [
                ('main', 'branch', ['refs/heads/main'],
                 {'pull_request', 'required_status_checks', 'non_fast_forward', 'deletion'}),
                ('release-tags-fixed', 'tag', ['refs/tags/v*'], {'update', 'deletion'}),
                ('release-tags-create', 'tag', ['refs/tags/v*'], {'creation'})]:
            with self.subTest(name=name):
                obj = rulesets[name]
                self.assertEqual(obj['target'], target)
                self.assertEqual(obj['enforcement'], 'active')
                self.assertEqual(obj['conditions'], {'ref_name': {'include': refs, 'exclude': []}})
                self.assertEqual({r['type'] for r in obj['rules']}, rules)
                self.assertEqual(obj['bypass_actors'],
                                 [{'actor_id': 5, 'actor_type': 'RepositoryRole', 'bypass_mode': 'always'}]
                                 if name == 'release-tags-create' else [])
        checks = next(r for r in rulesets['main']['rules'] if r['type'] == 'required_status_checks')
        self.assertEqual(checks['parameters']['required_status_checks'], [{'context': 'ci'}])

    def test_environments_have_exact_refs_and_no_reviewers(self):
        envs = {e['name']: e for e in self.config['environments']}
        self.assertEqual(set(envs), {'production', 'corpus-release'})
        for name, refs in [('production', [('v*', 'tag'), ('main', 'branch')]),
                           ('corpus-release', [('main', 'branch')])]:
            with self.subTest(name=name):
                self.assertEqual(envs[name]['settings']['reviewers'], [])
                self.assertEqual(envs[name]['settings']['deployment_branch_policy'],
                                 {'protected_branches': False, 'custom_branch_policies': True})
                self.assertEqual([(p['name'], p['type']) for p in envs[name]['refs']], refs)

    def test_apply_updates_rulesets_by_name_and_converges_environment_refs(self):
        self.assertTrue(Path(__file__).with_name('settings.py').exists(), 'settings helper missing')
        settings = importlib.import_module('settings')
        requests = []

        def request(method, path, body=None):
            requests.append((method, path, body))
            if method == 'GET' and '/rulesets?' in path:
                return [{'id': 17, 'name': 'main'}]
            if method == 'GET' and '/environments?' in path:
                return [{'name': 'production'}]
            if method == 'GET' and '/deployment-branch-policies?' in path:
                return [{'id': 21, 'name': 'main', 'type': 'branch'},
                        {'id': 22, 'name': '*', 'type': 'tag'}]
            return {}

        settings.apply(self.config, request)
        self.assertIn(('PUT', 'repos/dannyota/trachuan/rulesets/17', self.config['rulesets'][0]), requests)
        writes = [(method, path) for method, path, body in requests if method != 'GET']
        self.assertEqual(writes.count(('POST', 'repos/dannyota/trachuan/rulesets')), 2)
        self.assertIn(('DELETE', 'repos/dannyota/trachuan/environments/production/deployment-branch-policies/22'), writes)
        self.assertNotIn(('DELETE', 'repos/dannyota/trachuan/environments/production/deployment-branch-policies/21'), writes)

    def test_dry_run_prints_requests_and_never_executes_a_write(self):
        settings = importlib.import_module('settings')
        with patch('settings.subprocess.run') as run, redirect_stdout(io.StringIO()) as output:
            for method in ('PUT', 'POST', 'DELETE'):
                settings.gh_request(method, 'repos/dannyota/trachuan/rulesets/17', {'name': 'main'}, dry_run=True)
            run.assert_not_called()
        self.assertEqual(len(output.getvalue().splitlines()), 3)
        self.assertIn('PUT repos/dannyota/trachuan/rulesets/17', output.getvalue())

    def test_apply_again_does_not_create_duplicate_settings(self):
        settings = importlib.import_module('settings')
        requests = []

        def request(method, path, body=None):
            requests.append((method, path, body))
            if method == 'GET' and '/rulesets?' in path:
                return [obj | {'id': index} for index, obj in enumerate(self.config['rulesets'])]
            if method == 'GET' and '/environments?' in path:
                return self.config['environments']
            if method == 'GET' and '/deployment-branch-policies?' in path:
                name = path.split('/')[4]
                refs = next(e['refs'] for e in self.config['environments'] if e['name'] == name)
                return [obj | {'id': index} for index, obj in enumerate(refs)]
            return {}

        settings.apply(self.config, request)
        self.assertEqual({method for method, path, body in requests}, {'GET', 'PUT'})

    def test_enables_custom_policies_before_listing_them(self):
        settings = importlib.import_module('settings')
        enabled = set()

        def request(method, path, body=None):
            if method == 'GET' and '/rulesets?' in path:
                return []
            if method == 'GET' and '/environments?' in path:
                return [{'name': 'corpus-release', 'deployment_branch_policy': None}]
            if method == 'PUT' and '/environments/' in path:
                enabled.add(path)
            if method == 'GET' and '/deployment-branch-policies?' in path:
                self.assertIn(path.split('/deployment-branch-policies')[0], enabled)
                return []
            return {}

        settings.apply(self.config, request)


if __name__ == '__main__':
    unittest.main()
