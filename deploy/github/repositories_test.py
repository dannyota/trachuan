import importlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
import unittest.mock
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import patch

from corpus_manifest import create, read_manifest as read_corpus, sha256
from corpus_test_support import write_fixture
import manifest
import repositories

OLD, NEW = 'dannyota/trachuan', 'dntmakers/trachuan'
NAMES = ('server', 'web', 'queryembed', 'postgres')
DIGESTS = {name: 'sha256:' + 'b' * 64 for name in NAMES}
CADDY = 'docker.io/library/caddy@sha256:' + 'c' * 64


def listing(directory, value):
    path = Path(directory) / 'deploy-policy.json'
    path.write_text(json.dumps(dict(json.loads(repositories.POLICY.read_bytes()), repositories=value)))
    return patch.object(repositories, 'POLICY', path)


def one_repository(directory, name):
    return listing(directory, [name])


class WorkflowRepositoryTest(unittest.TestCase):
    def test_actions_value_or_required_flag(self):
        with patch.dict(os.environ, GITHUB_REPOSITORY=NEW):
            self.assertEqual(repositories.workflow(), NEW)
            self.assertEqual(repositories.workflow(NEW), NEW)
            with self.assertRaisesRegex(ValueError, 'differs'):
                repositories.workflow(OLD)
        with patch.dict(os.environ):
            os.environ.pop('GITHUB_REPOSITORY', None)
            self.assertEqual(repositories.workflow(OLD), OLD)
            with self.assertRaisesRegex(ValueError, '--repository'):
                repositories.workflow()
            for value in ('dannyota', 'a/b/c', 'owner/name\n', ''):
                with self.subTest(value=value), self.assertRaises(ValueError):
                    repositories.workflow(value)

    def test_listed_refuses_a_malformed_policy_list(self):
        for value in (OLD, {OLD: 1}, [], [OLD, NEW, 'third/trachuan'], [OLD, OLD], [OLD, None], ['bad name'], [OLD, 'a/b/c']):
            with self.subTest(value=value), tempfile.TemporaryDirectory() as directory, listing(directory, value), \
                    self.assertRaises(ValueError):
                repositories.listed()
        for value in ([NEW], [OLD, NEW]):
            with self.subTest(value=value), tempfile.TemporaryDirectory() as directory, listing(directory, value):
                self.assertEqual(repositories.listed(), value)

    def test_policy_lists_the_repositories_and_their_derived_image_names(self):
        policy = json.loads(repositories.POLICY.read_bytes())
        self.assertEqual(repositories.listed(), policy['repositories'])
        for name in policy['repositories']:
            self.assertEqual({image: repositories.package(name, image) for image in NAMES}, policy['images'][name])
        self.assertEqual(repositories.package(NEW, 'release'), 'ghcr.io/dntmakers/trachuan-release')
        for value in ('attacker/trachuan', None, ['dannyota/trachuan']):
            with self.subTest(value=value), self.assertRaises(ValueError):
                repositories.require_listed(value)


class UnlistedRepositoryWriteTest(unittest.TestCase):
    UNLISTED = ('dannyota/other', 'dntmakers/other', 'dntmakers/trachuan2')

    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)

    def refused(self, call, *mocks):
        for name in self.UNLISTED:
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, 'deploy policy'):
                call(name)
            for mock in mocks:
                mock.assert_not_called()

    def test_dispatch_commands_are_refused(self):
        corpus_dispatch = importlib.import_module('corpus_dispatch')
        frameworks_upload = importlib.import_module('frameworks_upload')
        self.refused(lambda name: corpus_dispatch.dispatch_command(name, 'vn', '20990102_1', 'a' * 64, 'b' * 64))
        self.refused(lambda name: frameworks_upload.dispatch_command('20990102_1', 'a' * 64, name))
        for name in (OLD, NEW):
            self.assertIn(name, corpus_dispatch.dispatch_command(name, 'vn', '20990102_1', 'a' * 64, ''))
            self.assertIn(name, frameworks_upload.dispatch_command('20990102_1', 'a' * 64, name))

    def test_corpus_upload_is_refused_before_any_work(self):
        upload = importlib.import_module('corpus_upload')
        with patch('corpus_upload.validated_inputs') as inputs, patch('corpus_upload.require_settled') as settled, \
                patch('corpus_upload.subprocess.run') as run, patch.dict(os.environ, OCI_S3_ENDPOINT='https://synthetic.invalid'):
            self.refused(lambda name: upload.upload('vn', '20990102_1', self.path, self.path / 'report.json', name),
                         inputs, settled, run)

    def test_frameworks_upload_is_refused_before_any_work(self):
        upload = importlib.import_module('frameworks_upload')
        with patch('frameworks_upload.validate_release') as validate, patch('frameworks_upload.subprocess.run') as run, \
                patch('frameworks_upload.private_directory') as private, patch('frameworks_upload.s3_endpoint') as endpoint:
            self.refused(lambda name: upload.run('20990102_1', False, name), validate, run, private, endpoint)
            self.refused(lambda name: upload.upload('20990102_1', self.path, {}, None, 'https://synthetic.invalid', name),
                         run, private)

    def test_image_publish_and_rollback_are_refused_before_any_write(self):
        publish = importlib.import_module('publish')
        with patch('publish.sign') as sign, patch('publish.find') as find, patch('publish.production') as production, \
                patch('publish.subprocess.run') as run:
            self.refused(lambda name: publish.release(self.path, 'refs/tags/v1.2.4', 'a' * 40, name), sign, production, run)
            self.refused(lambda name: publish.rollback(self.path, 'a' * 64, 'b' * 64, 9, 'refs/heads/main', 'c' * 40, name),
                         find, sign, run)
            self.refused(lambda name: publish.push(self.path, 'ghcr.io/x/y', 'v1', 'application/json', name), run)

    def test_corpus_publish_is_refused_before_any_write(self):
        publish = importlib.import_module('corpus_publish')
        with patch('corpus_publish.sign') as sign, patch('corpus_publish.require_signable') as signable, \
                patch('corpus_publish.read_manifest') as read, patch('publish.subprocess.run') as run:
            self.refused(lambda name: publish.publish('a' * 64, 'b' * 64, self.path, 'refs/heads/main', 'a' * 40, name),
                         sign, signable, read, run)

    def test_frameworks_publish_is_refused_before_any_write(self):
        module = importlib.import_module('frameworks_publish')
        with patch('frameworks_publish.sign') as sign, patch('frameworks_publish.validate_release') as validate, \
                patch('frameworks_publish.require_signed') as signed, patch('frameworks_publish.production') as production, \
                patch('frameworks_publish.subprocess.run') as run, patch('publish.subprocess.run') as push:
            self.refused(lambda name: module.prepare('20990102_1', 'a' * 64, self.path, 'refs/heads/main', 'a' * 40, 5, name),
                         validate, signed)
            self.refused(lambda name: module.publish(self.path, self.path, 'refs/heads/main', 'a' * 40, name),
                         sign, production, run, push)
            self.refused(lambda name: module.release('20990102_1', 'a' * 64, self.path, 'refs/heads/main', 'a' * 40, 5, name),
                         validate, sign, run, push)

    def test_settings_are_refused_before_any_request(self):
        settings = importlib.import_module('settings')
        request = unittest.mock.Mock(return_value=[])
        self.refused(lambda name: settings.apply(dict(rulesets=[], environments=[]), request, name), request)
        with patch('settings.apply') as apply, patch.dict(os.environ), patch('sys.argv', ['settings.py', '--dry-run',
                                                                                          '--repository', 'dannyota/other']):
            os.environ.pop('GITHUB_REPOSITORY', None)
            with self.assertRaisesRegex(ValueError, 'deploy policy'):
                settings.main()
            apply.assert_not_called()


class ImageManifestRepositoryTest(unittest.TestCase):
    def test_release_from_each_owner_names_that_owners_images(self):
        for name in (OLD, NEW):
            obj = manifest.read_release(manifest.release(name, 'a' * 40, 'refs/tags/v1.2.3', 4, DIGESTS, CADDY, b'b'))
            with self.subTest(name=name):
                self.assertEqual(obj['repository'], name)
                self.assertEqual(obj['images'], {image: repositories.package(name, image) + '@' + DIGESTS[image]
                                                 for image in NAMES})

    def test_unlisted_repository_or_other_owners_images_are_refused(self):
        with self.assertRaises(ValueError):
            manifest.release('attacker/trachuan', 'a' * 40, 'refs/tags/v1.2.3', 4, DIGESTS, CADDY, b'b')
        obj = json.loads(manifest.release(OLD, 'a' * 40, 'refs/tags/v1.2.3', 4, DIGESTS, CADDY, b'b'))
        with self.assertRaises(ValueError):
            manifest.read_release(manifest.canonical(dict(obj, repository=NEW)))
        mixed = json.loads(manifest.release(NEW, 'a' * 40, 'refs/tags/v1.2.3', 4, DIGESTS, CADDY, b'b'))
        mixed['images']['web'] = obj['images']['web']
        with self.assertRaises(ValueError):
            manifest.read_release(manifest.canonical(mixed))


class SigningRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.signing = importlib.import_module('signing')

    def test_verify_binds_the_given_repository_and_its_identity(self):
        for name in (OLD, NEW):
            for workflow, ref, release in (('images.yml', 'refs/tags/v1.2.3', True), ('rollback.yml', 'refs/heads/main', False),
                                           ('corpus-release.yml', 'refs/heads/main', False),
                                           ('frameworks-release.yml', 'refs/heads/main', False)):
                with self.subTest(name=name, workflow=workflow), patch('signing.subprocess.run') as run:
                    self.signing.verify('m.json', 's.json', name, workflow, ref, 'a' * 40, release=release)
                    command = run.call_args.args[0]
                    self.assertEqual(command[command.index('--certificate-github-workflow-repository') + 1], name)
                    certificate = 'https://github.com/' + name + '/.github/workflows/' + workflow + '@' + ref
                    if release:
                        self.assertRegex(certificate, command[command.index('--certificate-identity-regexp') + 1])
                        other = OLD if name == NEW else NEW
                        self.assertNotRegex(certificate.replace(name, other),
                                            command[command.index('--certificate-identity-regexp') + 1])
                    else:
                        self.assertEqual(command[command.index('--certificate-identity') + 1], certificate)

    def test_release_identity_escapes_the_repository_name(self):
        name = 'dot.owner/trachuan'
        with tempfile.TemporaryDirectory() as directory, listing(directory, [name]), patch('signing.subprocess.run') as run:
            self.signing.verify('m.json', 's.json', name, 'images.yml', 'refs/tags/v1.2.3', 'a' * 40, release=True)
        command = run.call_args.args[0]
        identity = command[command.index('--certificate-identity-regexp') + 1]
        self.assertRegex('https://github.com/dot.owner/trachuan/.github/workflows/images.yml@refs/tags/v1.2.3', identity)
        for widened in ('https://github.com/dotXowner/trachuan/.github/workflows/images.yml@refs/tags/v1.2.3',
                        'https://githubXcom/dot.owner/trachuan/.github/workflows/images.yml@refs/tags/v1.2.3'):
            self.assertNotRegex(widened, identity)

    def test_unlisted_repository_is_refused_before_cosign(self):
        with patch('signing.subprocess.run') as run:
            for name in ('attacker/trachuan', 'dannyota/other'):
                with self.subTest(name=name), self.assertRaises(ValueError):
                    self.signing.verify('m.json', 's.json', name, 'rollback.yml', 'refs/heads/main', 'a' * 40)
            with self.assertRaises(ValueError):
                self.signing.sign('m.json', 's.json', 'attacker/trachuan', 'rollback.yml', 'refs/heads/main', 'a' * 40)
            run.assert_not_called()


class EarlierReleaseRepositoryTest(unittest.TestCase):
    def test_earlier_release_is_verified_with_its_own_repository(self):
        registry = importlib.import_module('registry')
        document = manifest.release(NEW, 'a' * 40, 'refs/tags/v1.2.3', 4, DIGESTS, CADDY, b'b')
        layers = dict.fromkeys(('manifest.json', 'manifest.sigstore.json'), dict(digest='sha256:' + 'd' * 64, size=1))
        with tempfile.TemporaryDirectory() as work, \
                patch('registry.fetch_blob', side_effect=[document, b'signature']) as fetch, \
                patch('registry.verify') as verify:
            registry.verified_release('ghcr.io/dannyota/trachuan-release', layers, Path(work))
        self.assertEqual(verify.call_args.args[2:], (NEW, 'images.yml', 'refs/tags/v1.2.3', 'a' * 40))
        self.assertEqual(fetch.call_args.args[0], 'ghcr.io/dannyota/trachuan-release')


class PublishRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.publish = importlib.import_module('publish')
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)

    def release(self, name, sequence):
        return json.loads(manifest.release(name, 'a' * 40, f'refs/tags/v1.2.{sequence}', sequence, DIGESTS, CADDY, b'b'))

    def test_release_publishes_to_the_workflow_owner_with_its_source(self):
        (self.path / 'manifest.json').write_bytes(manifest.canonical(self.release(NEW, 4)))
        with patch('publish.sign') as sign, patch('publish.production', return_value=None) as production, \
                patch('publish.subprocess.run') as run:
            self.publish.release(self.path, 'refs/tags/v1.2.4', 'a' * 40, NEW)
        self.assertEqual(sign.call_args.args[2:], (NEW, 'images.yml', 'refs/tags/v1.2.4', 'a' * 40))
        self.assertEqual(production.call_args.args[0], 'ghcr.io/dntmakers/trachuan-release')
        push = run.call_args_list[0].args[0]
        self.assertEqual(push[:3], ['oras', 'push', 'ghcr.io/dntmakers/trachuan-release:v1.2.4'])
        self.assertIn('org.opencontainers.image.source=https://github.com/' + NEW, push)

    def test_release_claiming_another_repository_is_refused_before_signing(self):
        (self.path / 'manifest.json').write_bytes(manifest.canonical(self.release(OLD, 4)))
        with patch('publish.sign') as sign, patch('publish.production', return_value=None) as production, \
                patch('publish.subprocess.run') as run, self.assertRaisesRegex(ValueError, 'workflow source'):
            self.publish.release(self.path, 'refs/tags/v1.2.4', 'a' * 40, NEW)
        for mock in (sign, production, run):
            mock.assert_not_called()

    def test_rollback_refuses_every_target_while_the_policy_lists_two_repositories(self):
        self.assertEqual(len(repositories.listed()), 2)
        for name in (OLD, NEW):
            with self.subTest(name=name), patch('publish.find') as find, patch('publish.sign') as sign, \
                    patch('publish.subprocess.run') as run, self.assertRaisesRegex(ValueError, 'two repositories'):
                self.publish.rollback(self.path, 'a' * 64, 'b' * 64, 9, 'refs/heads/main', 'c' * 40, name)
            find.assert_not_called()
            sign.assert_not_called()
            run.assert_not_called()

    def test_rollback_refuses_a_target_from_another_repository_before_signing(self):
        current, target = self.release(NEW, 5), self.release(OLD, 4)
        with tempfile.TemporaryDirectory() as policy, one_repository(policy, NEW), \
                patch('publish.find', return_value={'a' * 64: current, 'b' * 64: target}), \
                patch('publish.sign') as sign, patch('publish.subprocess.run') as run, \
                self.assertRaisesRegex(ValueError, 'another repository'):
            self.publish.rollback(self.path, 'a' * 64, 'b' * 64, 9, 'refs/heads/main', 'c' * 40, NEW)
        sign.assert_not_called()
        run.assert_not_called()
        self.assertFalse((self.path / 'manifest.json').exists())

    def test_rollback_to_a_target_from_the_same_repository_signs_and_publishes(self):
        current, target = self.release(NEW, 5), self.release(NEW, 4)
        with tempfile.TemporaryDirectory() as policy, one_repository(policy, NEW), \
                patch('publish.find', return_value={'a' * 64: current, 'b' * 64: target}) as find, \
                patch('publish.sign') as sign, patch('publish.subprocess.run') as run:
            self.publish.rollback(self.path, 'a' * 64, 'b' * 64, 9, 'refs/heads/main', 'c' * 40, NEW)
        self.assertEqual(find.call_args.args[0], 'ghcr.io/dntmakers/trachuan-release')
        self.assertEqual(sign.call_args.args[2:], (NEW, 'rollback.yml', 'refs/heads/main', 'c' * 40))
        self.assertEqual(run.call_args.args[0][:3], ['oras', 'push', 'ghcr.io/dntmakers/trachuan-rollback:9'])


class CorpusRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)
        write_fixture(self.path)

    def test_corpus_manifest_from_each_owner_is_accepted_and_others_refused(self):
        for name in (OLD, NEW):
            data = create(name, 'a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json')
            with self.subTest(name=name):
                self.assertEqual(read_corpus(data)['repository'], name)
        with self.assertRaises(ValueError):
            create('attacker/trachuan', 'a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json')

    def test_earlier_corpus_manifest_is_verified_with_its_own_repository_from_the_workflow_channel(self):
        registry = importlib.import_module('corpus_registry')
        artifact = dict(artifactType='application/vnd.trachuan.corpus-release.v1',
                        layers=[dict(digest='sha256:' + 'b' * 64, size=20, annotations={'org.opencontainers.image.title': name})
                                for name in ('manifest.json', 'manifest.sigstore.json')])
        for workflow, signer in ((OLD, NEW), (NEW, OLD)):
            data = create(signer, 'a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json')
            responses = [b'{"tags":["vn-production"]}', ('sha256:' + 'c' * 64).encode(), json.dumps(artifact).encode()]
            with self.subTest(workflow=workflow), patch('corpus_registry.command', side_effect=responses) as command, \
                    patch('corpus_registry.fetch_blob', side_effect=[data, b'signature']) as fetch, \
                    patch('corpus_registry.verify') as verify:
                registry.production(workflow, 'vn', self.path / workflow.split('/')[0])
            channel = repositories.package(workflow, 'corpus-release')
            self.assertEqual(verify.call_args.args[2:], (signer, 'corpus-release.yml', 'refs/heads/main', 'a' * 40))
            self.assertEqual([call.args[0][-1] for call in command.call_args_list],
                             [channel, channel + ':vn-production', channel + '@sha256:' + 'c' * 64])
            self.assertEqual({call.args[0] for call in fetch.call_args_list}, {channel})

    def test_corpus_publish_uses_the_workflow_owners_channel(self):
        publish = importlib.import_module('corpus_publish')
        (self.path / 'manifest.json').write_bytes(
            create(NEW, 'a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json'))
        current = json.loads((self.path / 'previous-manifest.json').read_bytes()) | {'sequence': 3}
        with patch('corpus_publish.sign') as sign, patch('corpus_production.production', return_value=current) as production, \
                patch('corpus_production.existing_release', return_value=None) as existing, \
                patch('publish.subprocess.run') as run:
            publish.publish(sha256(self.path / 'report.json'), sha256(self.path / 'release.json'), self.path,
                            'refs/heads/main', 'a' * 40, NEW)
        self.assertEqual((production.call_args.args[0], existing.call_args.args[0]), (NEW, NEW))
        self.assertEqual(sign.call_args.args[2], NEW)
        push = run.call_args_list[0].args[0]
        self.assertEqual(push[:3], ['oras', 'push', 'ghcr.io/dntmakers/trachuan-corpus-release:vn-20990102_1'])
        self.assertIn('org.opencontainers.image.source=https://github.com/' + NEW, push)
        self.assertEqual(run.call_args_list[1].args[0],
                         ['oras', 'tag', 'ghcr.io/dntmakers/trachuan-corpus-release:vn-20990102_1', 'vn-production'])

    def test_corpus_publish_refuses_a_manifest_from_another_repository(self):
        publish = importlib.import_module('corpus_publish')
        (self.path / 'manifest.json').write_bytes(
            create(OLD, 'a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json'))
        with patch('corpus_publish.sign') as sign, patch('corpus_publish.require_signable', return_value={'sequence': 1}) as signable, \
                patch('publish.subprocess.run') as run, self.assertRaisesRegex(ValueError, 'workflow source'):
            publish.publish(sha256(self.path / 'report.json'), sha256(self.path / 'release.json'), self.path,
                            'refs/heads/main', 'a' * 40, NEW)
        for mock in (sign, signable, run):
            mock.assert_not_called()


class FrameworksChannelTest(unittest.TestCase):
    def setUp(self):
        support = importlib.import_module('frameworks_test_support')
        self.support, self.module = support, importlib.import_module('frameworks_publish')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        support.install_fakes(self.path / 'bin')
        (self.path / 'registry').mkdir()
        self.log = self.path / 'calls.jsonl'
        self.directory = self.path / 'release'
        self.value = support.state()
        support.write_objects(self.directory / 'objects', self.value)
        patcher = patch.dict(os.environ, dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'],
                                              FAKE_LOG=str(self.log), FAKE_REGISTRY=str(self.path / 'registry')))
        patcher.start()
        self.addCleanup(patcher.stop)

    def release(self, repository):
        with redirect_stdout(io.StringIO()):
            self.module.release(self.support.REL, self.support.report_hash(self.value), self.directory,
                                'refs/heads/main', self.support.COMMIT, 5, repository)

    def test_publish_and_read_use_the_workflow_owners_channel(self):
        self.release(NEW)
        channel, rel = 'ghcr.io/dntmakers/trachuan-corpus-release', self.support.REL
        log = [call['args'] for call in self.support.calls(self.log)]
        self.assertIn(['resolve', channel + ':frameworks-production'], log)
        push = next(args for args in log if args[0] == 'push')
        self.assertEqual(push[1], channel + ':frameworks-' + rel)
        self.assertIn('org.opencontainers.image.source=https://github.com/' + NEW, push)
        self.assertEqual(log[-1], ['tag', channel + ':frameworks-' + rel, 'frameworks-production'])
        verify = next(args for args in log if args[0] == 'verify-blob')
        self.assertEqual(verify[verify.index('--certificate-github-workflow-repository') + 1], NEW)
        registry = importlib.import_module('frameworks_registry')
        self.assertEqual(registry.release(NEW, rel, self.path / 'read')['repository'], NEW)
        reads = [call['args'] for call in self.support.calls(self.log)[len(log):]]
        self.assertEqual(reads[0], ['resolve', channel + ':frameworks-' + rel])
        self.assertTrue(all(args[-1].startswith(channel) for args in reads if args[0] in ('manifest', 'blob')))

    def test_publish_refuses_a_manifest_from_another_repository_before_signing(self):
        output = self.module.prepare(self.support.REL, self.support.report_hash(self.value), self.directory,
                                     'refs/heads/main', self.support.COMMIT, 5, OLD)
        with self.assertRaisesRegex(ValueError, 'workflow source'):
            self.module.publish(output, self.directory / 'production', 'refs/heads/main', self.support.COMMIT, NEW)
        self.assertFalse(any(call['args'][0] in ('sign-blob', 'push', 'tag') for call in self.support.calls(self.log)))


class FrameworksRepositoryTest(unittest.TestCase):
    def test_frameworks_manifest_from_each_owner_is_accepted_and_others_refused(self):
        frameworks = importlib.import_module('frameworks_manifest')
        support = importlib.import_module('frameworks_test_support')
        obj = support.manifest_object(support.state())
        for name in (OLD, NEW):
            with self.subTest(name=name):
                self.assertEqual(frameworks.validate_manifest(dict(obj, repository=name))['repository'], name)
        for name in ('attacker/trachuan', None):
            with self.subTest(name=name), self.assertRaises(ValueError):
                frameworks.validate_manifest(dict(obj, repository=name))


class EntryPointRepositoryTest(unittest.TestCase):
    ENTRIES = (('settings', 'apply', ['settings.py', '--dry-run'], 2),
               ('corpus_production', 'resign_differences', ['corpus_production.py', 'resign-check', 'vn', '20990102_1', '.'], 4),
               ('corpus_upload', 'upload', ['corpus_upload.py', 'vn', '20990102_1', '.', 'report.json'], 4),
               ('frameworks_upload', 'run', ['frameworks_upload.py', '20990102_1'], 2))

    def call(self, module, target, argv, environment):
        entry = importlib.import_module(module)
        returned = (dict(report=dict(sha256='a' * 64)), []) if target == 'resign_differences' else []
        with patch.dict(os.environ), patch('sys.argv', argv), patch(module + '.' + target, return_value=returned) as inner, \
                patch('frameworks_upload.guarded', side_effect=lambda function: function()), \
                redirect_stdout(io.StringIO()):
            os.environ.pop('GITHUB_REPOSITORY', None)
            os.environ.update(environment)
            entry.main()
        return inner

    def test_the_given_repository_reaches_the_call(self):
        for module, target, argv, index in self.ENTRIES:
            for environment in ({}, {'GITHUB_REPOSITORY': NEW}):
                with self.subTest(module=module, environment=environment):
                    inner = self.call(module, target, argv + ['--repository', NEW], environment)
                    self.assertEqual(inner.call_args.args[index], NEW)

    def test_a_repository_that_differs_from_actions_is_refused_before_the_call(self):
        for module, target, argv, index in self.ENTRIES:
            with self.subTest(module=module), self.assertRaisesRegex(ValueError, 'differs'):
                self.call(module, target, argv + ['--repository', NEW], {'GITHUB_REPOSITORY': OLD})


class SettingsAndCiRepositoryTest(unittest.TestCase):
    def test_settings_use_the_given_repository(self):
        settings = importlib.import_module('settings')
        requests = []

        def request(method, path, body=None):
            requests.append(path)
            return []

        settings.apply(dict(rulesets=[], environments=[]), request, NEW)
        self.assertTrue(requests)
        self.assertTrue(all(path.startswith('repos/' + NEW + '/') for path in requests))

    def test_settings_require_repository_outside_actions(self):
        settings = importlib.import_module('settings')
        with patch.dict(os.environ), patch('sys.argv', ['settings.py', '--dry-run']), \
                patch('settings.apply') as apply, redirect_stderr(io.StringIO()):
            os.environ.pop('GITHUB_REPOSITORY', None)
            with self.assertRaises((SystemExit, ValueError)):
                settings.main()
            apply.assert_not_called()
        with patch.dict(os.environ), patch('sys.argv', ['settings.py', '--dry-run', '--repository', NEW]), \
                patch('settings.apply') as apply:
            os.environ.pop('GITHUB_REPOSITORY', None)
            settings.main()
        self.assertEqual(apply.call_args.args[2], NEW)

    def test_wait_ci_reads_the_workflow_repository(self):
        wait_ci = importlib.import_module('wait_ci')
        with patch.dict(os.environ, GITHUB_REPOSITORY=NEW, GITHUB_TOKEN='synthetic'), \
                patch('wait_ci.urlopen') as urlopen:
            urlopen.return_value.__enter__.return_value = io.BytesIO(b'{"workflow_runs":[]}')
            wait_ci.fetch_runs('a' * 40)
        self.assertTrue(urlopen.call_args.args[0].full_url.startswith(
            'https://api.github.com/repos/' + NEW + '/actions/workflows/ci.yml/runs?'))


if __name__ == '__main__':
    unittest.main()
