import fcntl
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import unittest

from repositories_test_support import enable_dual_policy
from frameworks_test_support import (OBJECTS, PREVIOUS, REL, SENTINEL, STEPS, bind, calls, digest, encoded, install_fakes,
                                     manifest_object, object_bytes, publish_tag, rebind_build, report_hash, state, write_run)

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/corpus-upload.sh'
BUCKET = 'trachuan-frameworks-releases'
ENDPOINT = 'https://synthetic.compat.objectstorage.ap-singapore-1.oraclecloud.com'


class UploadCase(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(os.path.realpath(self.temp.name))
        self.home = self.path / 'home'
        self.home.mkdir()
        self.work = self.path / 'work'
        self.base = self.work / REL
        self.base.mkdir(parents=True)
        self.value = state()
        write_run(self.base, self.value)
        install_fakes(self.path / 'bin')
        self.log = self.path / 'calls.jsonl'
        self.remote = self.path / 'remote'
        self.remote.mkdir()
        self.registry = self.path / 'registry'
        self.registry.mkdir()
        self.outer_tmp = self.path / 'outer-tmp'
        self.outer_tmp.mkdir()
        self.env = dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'], HOME=str(self.home),
                        OCI_S3_ENDPOINT=ENDPOINT, FRAMEWORKS_WORK_DIR=str(self.work), TMPDIR=str(self.outer_tmp),
                        FAKE_LOG=str(self.log), FAKE_REMOTE=str(self.remote), FAKE_REGISTRY=str(self.registry))

    def upload(self, *flags, rel=REL, repository='dannyota/trachuan', **extra):
        return subprocess.run(['bash', str(SCRIPT), *flags, '--repository', repository, 'frameworks', rel], env=self.env | extra,
                              capture_output=True, text=True)

    def save(self, value=None):
        write_run(self.base, value or self.value)

    def calls(self):
        return calls(self.log)

    def stored(self, name, rel=REL):
        return self.remote / BUCKET / 'frameworks' / rel / name

    def assert_clean(self):
        self.assertFalse((self.base / '.upload').exists())
        self.assertEqual(list(self.outer_tmp.iterdir()), [])

    def assert_refused(self, result):
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        self.assertNotIn(SENTINEL, result.stdout + result.stderr)
        self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))
        self.assert_clean()

    def assert_nothing_uploaded(self):
        self.assertFalse(any({'put-object', 'create-multipart-upload'} & set(call['args']) for call in self.calls()))

    def publish_previous(self):
        earlier = state(PREVIOUS)
        publish_tag(self.registry, 'frameworks-production', manifest_object(earlier))
        report = object_bytes(earlier)['report.json']
        path = self.stored('report.json', PREVIOUS)
        path.parent.mkdir(parents=True)
        path.write_bytes(report)
        self.value['report']['previous'] = dict(rel=PREVIOUS, report_sha256=digest(report))
        self.save()
        return report


class FrameworksUploadTest(UploadCase):
    def test_first_release_uploads_verifies_and_dispatches(self):
        result = self.upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        for name, data in object_bytes(self.value).items():
            self.assertEqual(self.stored(name).read_bytes(), data)
        log = self.calls()
        puts = [i for i, call in enumerate(log) if 'put-object' in call['args']]
        heads = [i for i, call in enumerate(log) if 'head-object' in call['args']]
        self.assertEqual((len(puts), len(heads)), (5, 5))
        self.assertLess(max(puts), min(heads))
        self.assertEqual(log[-1]['name'], 'gh')
        self.assertEqual(log[-1]['args'], ['workflow', 'run', 'frameworks-release.yml', '--repo', 'dannyota/trachuan',
                                           '--ref', 'main', '-f', f'rel={REL}',
                                           '-f', f'report_sha256={report_hash(self.value)}'])
        self.assertEqual([call['args'] for call in log if call['name'] == 'oras'],
                         [['resolve', 'ghcr.io/dannyota/trachuan-corpus-release:frameworks-production']])
        self.assertFalse(any(call['name'] == 'cosign' for call in log))
        for call in log:
            if call['name'] == 'aws' and '--version' not in call['args']:
                args = call['args']
                self.assertEqual(args[:5], ['--profile', 'trachuan-release-upload', '--endpoint-url', ENDPOINT, '--no-cli-pager'])
                self.assertEqual(args[args.index('--bucket') + 1], BUCKET)
                self.assertEqual(call['checksum'], 'when_required')
                self.assertIn('addressing_style = path', call['config'])
                self.assertIn('[profile trachuan-release-upload]', call['config'])
        self.assertIn('aws-cli/2.40.0', result.stdout)
        self.assertNotIn(SENTINEL, result.stdout + result.stderr)
        self.assert_clean()
        lock = self.home / '.cache/trachuan'
        self.assertEqual(lock.stat().st_mode & 0o777, 0o700)
        self.assertTrue((lock / 'corpus-upload.lock').exists())

    def test_prefix_is_checked_with_the_law_query_before_any_upload(self):
        self.assertEqual(self.upload().returncode, 0)
        log = self.calls()
        query = next(i for i, call in enumerate(log) if 'list-objects-v2' in call['args'])
        self.assertEqual(log[query]['args'][5:], ['s3api', 'list-objects-v2', '--bucket', BUCKET, '--prefix',
                                                  f'frameworks/{REL}/', '--max-keys', '1', '--query', 'KeyCount',
                                                  '--output', 'text'])
        self.assertLess(query, min(i for i, call in enumerate(log) if 'put-object' in call['args']))

    def test_next_release_checks_the_active_signed_report(self):
        report = self.publish_previous()
        result = self.upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        log = self.calls()
        verify = next(call['args'] for call in log if call['name'] == 'cosign')
        self.assertIn('https://github.com/dannyota/trachuan/.github/workflows/frameworks-release.yml@refs/heads/main', verify)
        fetch = next(call['args'] for call in log if 'get-object' in call['args'])
        self.assertEqual(fetch[fetch.index('--key') + 1], f'frameworks/{PREVIOUS}/report.json')
        self.assertEqual(fetch[fetch.index('--range') + 1], f'bytes=0-{len(report) - 1}')
        self.assertLess(log.index(next(c for c in log if 'get-object' in c['args'])),
                        log.index(next(c for c in log if 'put-object' in c['args'])))

    def test_dry_run_checks_local_inputs_only(self):
        result = self.upload('--dry-run')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [])
        self.assertFalse((self.home / '.cache').exists())
        self.assert_clean()
        for name in OBJECTS:
            self.assertIn(f's3://{BUCKET}/frameworks/{REL}/{name}', result.stdout)
        self.assertIn(f'gh workflow run frameworks-release.yml --repo dannyota/trachuan --ref main -f rel={REL} '
                      f'-f report_sha256={report_hash(self.value)}', result.stdout)
        moved = self.upload('--dry-run', repository='dntmakers/trachuan')
        self.assertEqual(moved.returncode, 0, moved.stderr)
        self.assertIn('gh workflow run frameworks-release.yml --repo dntmakers/trachuan --ref main', moved.stdout)
        self.assertEqual(self.calls(), [])
        self.assertNotIn(SENTINEL, result.stdout)
        self.value['report']['quality_pass'] = False
        self.save()
        self.assert_refused(self.upload('--dry-run'))
        self.assertEqual(self.calls(), [])

    def test_every_local_refusal_happens_before_any_client_call(self):
        changes = [lambda v: v['report'].update(quality_pass=False), lambda v: v['report'].update(dump_sha256='0' * 64),
                   lambda v: (v['build']['dump'].update(sha256='0' * 64), rebind_build(v)),
                   lambda v: v['report'].update(build_manifest_sha256='0' * 64),
                   lambda v: v['report'].update(rel='20990103'), lambda v: v['row'].update(rel='20990103'),
                   lambda v: v['report']['recipe_hashes'].update(build='0' * 64),
                   lambda v: v['report']['recipe_hashes'].update(evaluate='0' * 64),
                   lambda v: v['row']['extensions'].pop('ltree'),
                   lambda v: v['row']['retrieval_recipe'].update(note=SENTINEL)]
        changes += [(lambda name: lambda v: v['report']['recipe_hashes'].update({name: '0' * 64}))(name) for name in STEPS]
        for index, change in enumerate(changes):
            with self.subTest(index=index):
                value = state()
                change(value)
                self.save(value)
                self.assert_refused(self.upload())
                self.assertEqual(self.calls(), [])

    def test_missing_inputs_and_invalid_identity_refuse(self):
        for name, relative in OBJECTS.items():
            with self.subTest(name=name):
                self.save()
                (self.base / relative).unlink()
                self.assert_refused(self.upload())
                self.assertEqual(self.calls(), [])
        self.save()
        for rel, code in (('2099010', 2), ('20990102_123', 2), ('../x', 2)):
            with self.subTest(rel=rel):
                self.assertEqual(self.upload(rel=rel).returncode, code)
        self.assert_refused(self.upload(rel='20990103'))
        env = dict(self.env)
        del env['FRAMEWORKS_WORK_DIR']
        self.assert_refused(subprocess.run(['bash', str(SCRIPT), '--repository', 'dannyota/trachuan', 'frameworks', REL], env=env,
                                           capture_output=True, text=True))
        self.assert_refused(self.upload(OCI_S3_ENDPOINT='http://synthetic.example'))

    def test_previous_refusals_stop_before_any_upload(self):
        named = dict(rel=PREVIOUS, report_sha256='b' * 64)
        self.value['report']['previous'] = named
        self.save()
        self.assert_refused(self.upload())
        self.assert_nothing_uploaded()
        report = self.publish_previous()
        good = dict(self.value['report']['previous'])
        cases = [(None, {}), (dict(good, rel='20990100'), {}), (dict(good, report_sha256='0' * 64), {}),
                 (good, dict(FAKE_BAD_SIGNATURE='1')), (good, dict(FAKE_RESOLVE_ERROR='Error: unauthorized'))]
        for previous, extra in cases:
            with self.subTest(previous=previous, extra=extra):
                self.log.unlink(missing_ok=True)
                self.value['report']['previous'] = previous
                self.save()
                self.assert_refused(self.upload(**extra))
                self.assert_nothing_uploaded()
        self.value['report']['previous'] = good
        self.save()
        self.log.unlink(missing_ok=True)
        self.stored('report.json', PREVIOUS).write_bytes(report + b' ')
        self.assert_refused(self.upload())
        self.assert_nothing_uploaded()

    def test_existing_prefix_and_old_cli_upload_nothing(self):
        self.assert_refused(self.upload(FAKE_AWS_VERSION='1.44.0'))
        self.assert_nothing_uploaded()
        self.stored('frameworks.dump').parent.mkdir(parents=True)
        self.stored('frameworks.dump').write_bytes(b'existing')
        self.log.unlink()
        self.assert_refused(self.upload())
        self.assert_nothing_uploaded()
        self.assertEqual(self.stored('frameworks.dump').read_bytes(), b'existing')

    def test_receipt_changes_and_failures_prevent_dispatch_and_clean_up(self):
        for extra in (dict(FAKE_BAD_SIZE='report.json'), dict(FAKE_REPLACE_AFTER_UPLOAD='report.json'),
                      dict(FAKE_FAIL_ON='put-object'), dict(FAKE_HEAD_ETAGS=json.dumps({'build.json': '"other"'}))):
            with self.subTest(extra=extra):
                shutil.rmtree(self.remote)
                self.remote.mkdir()
                self.log.unlink(missing_ok=True)
                self.assert_refused(self.upload(**extra))

    def test_file_replaced_after_validation_is_not_uploaded_or_dispatched(self):
        for name, relative in OBJECTS.items():
            for grow in ('', '1'):
                with self.subTest(name=name, grow=grow):
                    self.save()
                    shutil.rmtree(self.remote)
                    self.remote.mkdir()
                    self.log.unlink(missing_ok=True)
                    self.assert_refused(self.upload(FAKE_CHANGE_FILE=str(self.base / relative), FAKE_CHANGE_GROW=grow))
                    self.assertFalse(self.stored(name).exists())

    def test_dispatch_failure_is_reported_once_and_cleans_up(self):
        result = self.upload(FAKE_FAIL_DISPATCH='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(sum(call['name'] == 'gh' for call in self.calls()), 1)
        self.assert_clean()

    def test_law_and_frameworks_uploads_share_the_lock(self):
        directory = self.home / '.cache/trachuan'
        directory.mkdir(parents=True)
        with (directory / 'corpus-upload.lock').open('w') as stream:
            fcntl.flock(stream, fcntl.LOCK_EX)
            result = self.upload()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Another corpus upload holds the local lock', result.stderr)
        self.assertEqual(self.calls(), [])
        self.assertTrue(self.upload('--dry-run').returncode == 0)

    def test_law_argument_forms_are_untouched(self):
        for arguments in ([], ['vn'], ['frameworks'], ['frameworks', REL, 'extra'], ['frameworks', REL, 'x', 'y', 'z']):
            with self.subTest(arguments=arguments):
                result = subprocess.run(['bash', str(SCRIPT), '--repository', 'dannyota/trachuan', *arguments], env=self.env,
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 2)
                self.assertIn('usage: corpus-upload.sh [--dry-run] --repository <owner/name> <cc> <rel> <build-dir> <report.json>',
                              result.stderr)
        for arguments in (['frameworks', REL], ['--dry-run', 'frameworks', REL], ['--repository', 'frameworks', REL],
                          ['--repository', 'bad name', 'frameworks', REL], ['vn', '20990102_1', 'b', 'c']):
            with self.subTest(arguments=arguments):
                result = subprocess.run(['bash', str(SCRIPT), *arguments], env=self.env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 2)
                self.assertIn('--repository <owner/name>', result.stderr)
        result = subprocess.run(['bash', str(SCRIPT), '--repository', 'dannyota/trachuan', 'frameworks', '20990102_1', 'b', 'c'], env=self.env,
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(self.calls(), [])


class FrameworksConfinementTest(UploadCase):
    def test_every_child_runs_with_the_private_temporary_directory(self):
        self.publish_previous()
        result = self.upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        upload = str(self.base / '.upload')
        log = self.calls()
        self.assertEqual({call['name'] for call in log}, {'aws', 'oras', 'cosign', 'gh'})
        for call in log:
            self.assertEqual(call['tmpdir'], upload, call['args'])
            self.assertEqual(call['tmp_mode'], '0o700', call['args'])
            self.assertEqual(call['core'][0], 0, call['args'])
        config = {call['config_path'] for call in log if call['name'] == 'aws'}
        self.assertEqual({Path(path).parent for path in config}, {Path(upload)})
        for call in log:
            if call['name'] == 'cosign':
                self.assertTrue(all(arg.startswith(upload) for arg in call['args'] if '/' in arg and arg.startswith('/')))
        self.assertTrue(next(c for c in log if 'get-object' in c['args'])['args'][-1].startswith(upload + '/'))
        bodies = [c['args'][c['args'].index('--body') + 1] for c in log if 'put-object' in c['args']]
        self.assertEqual(len(bodies), 5)
        self.assertTrue(all(body.startswith(upload + '/') for body in bodies))
        self.assert_clean()

    def test_multipart_parts_stay_under_the_private_directory(self):
        self.value['dump'] = b'x' * (64 * 1024**2 + 1)
        bind(self.value)
        self.save()
        result = self.upload()
        self.assertEqual(result.returncode, 0, result.stderr)
        parts = [call['args'] for call in self.calls() if 'upload-part' in call['args']]
        self.assertEqual(len(parts), 2)
        for args in parts:
            self.assertTrue(args[args.index('--body') + 1].startswith(str(self.base / '.upload') + '/'))
        self.assertEqual(digest(self.stored('frameworks.dump').read_bytes()), digest(self.value['dump']))
        self.assert_clean()

    def test_stale_private_directory_is_replaced(self):
        stale = self.base / '.upload'
        stale.mkdir(mode=0o755)
        (stale / 'leftover').write_bytes(b'stale')
        self.assertEqual(self.upload().returncode, 0)
        self.assert_clean()

    def test_termination_during_upload_removes_the_private_directory(self):
        result = self.upload(FAKE_TERMINATE_ON='put-object')
        self.assertEqual(result.returncode, 128 + signal.SIGTERM)
        self.assert_clean()
        self.assertFalse(any(call['name'] == 'gh' for call in self.calls()))

    def test_symlinked_private_directory_is_refused_and_its_target_kept(self):
        target = self.base / 'build'
        (self.base / '.upload').symlink_to(target)
        result = self.upload()
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue((target / 'frameworks.dump').exists())
        self.assert_nothing_uploaded()

    def test_synced_work_directories_are_refused_at_every_level(self):
        documents = self.home / 'Documents'
        documents.mkdir()
        inside = documents / 'work'
        shutil.copytree(self.work, inside)
        link = self.path / 'alias'
        link.symlink_to(inside)
        marked = self.path / 'marked'
        shutil.copytree(self.work, marked / 'work')
        (marked / '.dropbox').write_text('')
        for work in (inside, link, marked / 'work'):
            with self.subTest(work=work):
                result = self.upload(FRAMEWORKS_WORK_DIR=str(work))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('sync folder refused', result.stderr)
                self.assertEqual(self.calls(), [])
                self.assertFalse((work / REL / '.upload').exists())

    def test_synced_release_and_step_directories_are_refused(self):
        drive = self.home / 'Dropbox'
        drive.mkdir()
        for relative in ('build', 'evaluate', 'release'):
            with self.subTest(relative=relative):
                shutil.rmtree(self.base)
                self.base.mkdir()
                write_run(self.base, self.value)
                moved = drive / relative
                shutil.rmtree(moved, ignore_errors=True)
                shutil.move(self.base / relative, moved)
                (self.base / relative).symlink_to(moved)
                result = self.upload()
                self.assertIn('sync folder refused', result.stderr)
                self.assertEqual(self.calls(), [])
        shutil.rmtree(self.base)
        moved = drive / REL
        write_run(moved, self.value)
        self.base.symlink_to(moved)
        self.assertIn('sync folder refused', self.upload().stderr)
        self.assertEqual(self.calls(), [])

    def test_symlinked_input_file_is_refused(self):
        source = self.base / 'build/build.json'
        moved = self.path / 'elsewhere.json'
        source.rename(moved)
        source.symlink_to(moved)
        self.assert_refused(self.upload())
        self.assertEqual(self.calls(), [])

    def test_output_holds_identifiers_only_when_a_json_input_is_invalid(self):
        (self.base / 'evaluate/report.json').write_bytes(encoded(dict(note=SENTINEL))[:-1])
        result = self.upload()
        self.assert_refused(result)
        self.assertEqual(len(result.stderr.strip().splitlines()), 1)


def setUpModule():
    global SCRIPT
    enable_dual_policy()
    legacy_checkout = tempfile.TemporaryDirectory()
    unittest.addModuleCleanup(legacy_checkout.cleanup)
    repo = Path(legacy_checkout.name)
    (repo / 'scripts').mkdir()
    shutil.copyfile(SCRIPT, repo / 'scripts/corpus-upload.sh')
    shutil.copytree(ROOT / 'deploy/github', repo / 'deploy/github', ignore=shutil.ignore_patterns('__pycache__'))
    (repo / 'deploy/host').mkdir()
    policy = json.loads((ROOT / 'deploy/host/deploy-policy.json').read_bytes())
    policy['repositories'] = ['dannyota/trachuan', 'dntmakers/trachuan']
    (repo / 'deploy/host/deploy-policy.json').write_text(json.dumps(policy))
    unittest.addModuleCleanup(globals().__setitem__, 'SCRIPT', SCRIPT)
    SCRIPT = repo / 'scripts/corpus-upload.sh'


if __name__ == '__main__':
    unittest.main()
