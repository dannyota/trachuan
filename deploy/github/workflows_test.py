from pathlib import Path
import re
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[2]


class WorkflowsTest(unittest.TestCase):
    def test_opentofu_ci_rules(self):
        text = (ROOT / '.github/workflows/ci.yml').read_text()
        preflight = text.split('\n  schema:\n', 1)[0]
        for line in ('tofu: ${{ steps.paths.outputs.tofu }}', 'tofu=true', 'tofu=false',
                     'deploy/oci/*|.github/workflows/ci.yml) tofu=true ;;', '"tofu=$tofu"'):
            self.assertIn(line, preflight)
        self.assertNotIn('deploy/oci/*) ;;', preflight)
        job = text.split('\n  opentofu:\n', 1)[1].split('\n  ci:\n', 1)[0]
        for line in ('needs: [preflight]', "if: needs.preflight.outputs.tofu == 'true'",
                     'runs-on: ubuntu-24.04', 'timeout-minutes: 5', 'contents: read',
                     'working-directory: deploy/oci', 'actions/checkout@',
                     'opentofu/setup-opentofu@a1320f892987e89d278cc92dc5adc984fb93aca4 # v2.0.2',
                     'tofu_version: 1.13.9', 'tofu_wrapper: false',
                     'tofu fmt -check -recursive', 'tofu init -backend=false -input=false -lockfile=readonly',
                     'tofu validate -no-color'):
            self.assertIn(line, job)
        ci = text.split('\n  ci:\n', 1)[1]
        for line in ('needs: [preflight, static, integration, e2e, docembed, agenteval, queryembed, schema, web, opentofu]',
                     'TOFU_NEEDED: ${{ needs.preflight.outputs.tofu }}', 'TOFU: ${{ needs.opentofu.result }}',
                     'if [ "$TOFU_NEEDED" = true ]; then', 'test "$TOFU" = success',
                     'elif [ "$TOFU_NEEDED" = false ]; then', 'test "$TOFU" = skipped'):
            self.assertIn(line, ci)

    def test_registry_login_lasts_through_attestations_and_always_logs_out(self):
        text = (ROOT / '.github/workflows/images.yml').read_text().split('\n  release:\n', 1)[0]
        steps = re.split(r'\n      - ', text)[1:]
        logout = [index for index, step in enumerate(steps) if 'docker logout ghcr.io' in step]
        attestations = [index for index, step in enumerate(steps) if 'actions/attest-build-provenance@' in step]
        self.assertEqual(len(logout), 1)
        self.assertEqual(len(attestations), 4)
        self.assertGreater(logout[0], max(attestations))
        self.assertEqual(logout[0], len(steps) - 1)
        self.assertIn('if: always()', steps[logout[0]])

    def test_release_gate_and_registry_attestations(self):
        text = (ROOT / '.github/workflows/images.yml').read_text()
        self.assertIn('\n  release:\n', text)
        release = text.split('\n  release:\n', 1)[1]
        for line in ('    needs: images', "    if: github.event_name == 'push'",
                     '    environment: production', '    timeout-minutes: 75',
                     '      group: release-production', '      cancel-in-progress: false',
                     '      queue: max',
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

    def test_workflows_name_no_owner_and_publish_under_the_repository_owner(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            text = path.read_text()
            for owner in ('dannyota', 'dntmakers'):
                self.assertNotIn(owner, text, path.name)
            self.assertEqual(set(re.findall(r'ghcr\.io/(?:\$\{\{[^}]*\}\}|\S)*', text)) - {'ghcr.io/${{ github.repository_owner }}'},
                             set(), path.name)
        images = (ROOT / '.github/workflows/images.yml').read_text().split('\n  release:\n', 1)[0]
        self.assertIn('\n      REGISTRY: ghcr.io/${{ github.repository_owner }}\n', images)
        self.assertEqual(re.findall(r'subject-name: (.+)', images),
                         ['${{ env.REGISTRY }}/trachuan-' + name for name in ('server', 'web', 'queryembed', 'postgres')])
        self.assertEqual(len(re.findall(r'\$REGISTRY/trachuan-', images)), 9)

    def test_images_job_checks_the_repository_policy_before_building_or_pushing(self):
        steps = yaml.safe_load((ROOT / '.github/workflows/images.yml').read_text())['jobs']['images']['steps']
        guard = [index for index, step in enumerate(steps) if 'write_repository()' in step.get('run', '')]
        self.assertEqual(len(guard), 1)
        self.assertEqual(steps[guard[0] - 1].get('uses', '').split('@')[0], 'actions/checkout')
        self.assertEqual(steps[guard[0]]['run'].strip(),
                         "PYTHONPATH=deploy/github python3 -c 'from repositories import write_repository; write_repository()'")
        self.assertFalse(any('secrets.' in str(step.get('env', '')) for step in steps[:guard[0] + 1]))
        for index, step in enumerate(steps):
            if any(word in step.get('run', '') for word in ('docker build', 'docker login', 'docker push')) or \
                    str(step.get('uses', '')).startswith(('actions/attest', 'docker/login')):
                self.assertGreater(index, guard[0], step.get('name'))

    def test_rollback_workflow_publishes_only_through_publish_py_rollback(self):
        workflow = yaml.safe_load((ROOT / '.github/workflows/rollback.yml').read_text())
        self.assertEqual(list(workflow), ['name', True, 'permissions', 'concurrency', 'jobs'])
        self.assertEqual(list(workflow['jobs']), ['rollback'])
        job = workflow['jobs']['rollback']
        self.assertEqual(set(job), {'if', 'environment', 'runs-on', 'timeout-minutes', 'permissions', 'env', 'steps'})
        self.assertEqual(sorted(job['env']), ['CURRENT', 'PYTHONDONTWRITEBYTECODE', 'TARGET'])
        steps = job['steps']
        self.assertEqual([step.get('uses', '').split('@')[0] for step in steps if 'uses' in step], ['actions/checkout'])
        self.assertTrue(all(set(step) <= {'name', 'uses', 'with', 'run', 'env'} for step in steps))
        self.assertEqual([step.get('with') for step in steps], [None, {'persist-credentials': False}, None, None])
        self.assertEqual([sorted(step.get('env', {})) for step in steps], [[], [], [], ['ACTOR', 'GITHUB_TOKEN']])
        lines = [line.strip() for step in steps for line in step.get('run', '').splitlines() if line.strip()]
        self.assertEqual(lines, [
            '[[ $GITHUB_REF == refs/heads/main ]]',
            '[[ $CURRENT =~ ^[0-9a-f]{64}$ && $TARGET =~ ^[0-9a-f]{64}$ ]]',
            'echo "RELEASE_DIR=$RUNNER_TEMP/trachuan-rollback" >> "$GITHUB_ENV"',
            'bash deploy/github/install-tools.sh',
            'printf \'%s\' "$GITHUB_TOKEN" | oras login ghcr.io --username "$ACTOR" --password-stdin',
            "trap 'oras logout ghcr.io' EXIT",
            'python3 deploy/github/publish.py rollback "$RELEASE_DIR"',
        ])

    def test_cache_prune_schedule_and_permissions(self):
        path = ROOT / '.github/workflows/cache-prune.yml'
        self.assertTrue(path.exists())
        text = path.read_text()
        self.assertRegex(text, r"schedule:\s*\n\s*- cron: ['\"]\d+ \d+ \* \* \*['\"]")
        self.assertIn('workflow_dispatch:', text)
        self.assertRegex(text, r'(?m)^permissions:\n  actions: write\n  contents: read\n')
        self.assertEqual(re.findall(r'(?m)^[ \t]+permissions:', text), [])
        self.assertEqual(re.findall(r'(?m)^\s+([\w-]+): write$', text), ['actions'])
        self.assertIn('deploy/github/cache_prune.py', text)

    def test_actions_are_pinned(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            for action in re.findall(r'uses: (\S+)', path.read_text()):
                self.assertRegex(action, r'^[\w./-]+@[0-9a-f]{40}$')


if __name__ == '__main__':
    unittest.main()
