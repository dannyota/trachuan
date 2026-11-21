import base64
import copy
import json
from pathlib import Path
import re
import tempfile
import unittest

from updater import deploy_corpus_verify as corpus
from updater import deploy_frameworks_verify as frameworks
from updater import deploy_verify as v
from updater.deploy_io import canonical, CommandError
from updater.verification_test import statement


HOST = Path(__file__).parents[1]
FIXTURES = Path(__file__).with_name('fixtures')
OLD, NEW = 'dannyota/trachuan', 'dntmakers/trachuan'
WORKFLOWS = dict(release='images.yml', build='images.yml', rollback='rollback.yml', corpus='corpus-release.yml',
                 frameworks='frameworks-release.yml')


def frameworks_identity(repository):
    return '^' + re.escape('https://github.com/' + repository + '/.github/workflows/frameworks-release.yml') + '@refs/heads/main$'


def rules():
    value = keyed_policy(v.policy(HOST / 'deploy-policy.json'), [OLD, NEW])
    return dict(value, frameworks=dict(repository='ghcr.io/dannyota/trachuan-corpus-release',
                                       identity={name: frameworks_identity(name) for name in value['repositories']},
                                       namespace='syntheticnamespace', bucket='synthetic-frameworks', min_release_floor=40))


def keyed_policy(base, names):
    value = dict(base, repositories=names)
    for kind, workflow in WORKFLOWS.items():
        if kind == 'frameworks':
            continue
        suffix = r'@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' if workflow == 'images.yml' else '@refs/heads/main$'
        value[kind + '_identity'] = {name: '^' + re.escape('https://github.com/' + name + '/.github/workflows/' + workflow) + suffix
                                     for name in names}
    value['images'] = {name: {image: 'ghcr.io/' + name.split('/')[0] + '/trachuan-' + image
                              for image in ('server', 'web', 'queryembed', 'postgres')} for name in names}
    return value


def moved(obj, repository):
    text = json.dumps(obj).replace('ghcr.io/dannyota/', 'ghcr.io/' + repository.split('/')[0] + '/')
    return dict(json.loads(text), repository=repository)


class FakeCosign:
    def __init__(self, repository, workflow, ref):
        self.certificate = 'https://github.com/' + repository + '/.github/workflows/' + workflow + '@' + ref
        self.repository = repository
        self.calls = []

    def __call__(self, args, **kwargs):
        self.calls.append(args)
        identity = args[args.index('--certificate-identity-regexp') + 1]
        claimed = args[args.index('--certificate-github-workflow-repository') + 1]
        if re.fullmatch(identity, self.certificate) is None or claimed != self.repository:
            raise CommandError('command failed', returncode=1)
        return b''

    def flags(self, flag):
        return [args[args.index(flag) + 1] for args in self.calls]


class PolicyRepositoriesTest(unittest.TestCase):
    def setUp(self):
        self.base = keyed_policy(json.loads((HOST / 'deploy-policy.json').read_bytes()), [OLD, NEW])

    def load(self, value):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'policy.json'
            path.write_bytes(canonical(value))
            return v.policy(path)

    def test_shipped_policy_trusts_only_new_owner_at_release_b_floor(self):
        value = v.policy(HOST / 'deploy-policy.json')
        self.assertEqual(value['repositories'], [NEW])
        self.assertEqual(value['release_floor'], 66)
        self.assertNotIn('frameworks', value)
        for key in ('release_identity', 'build_identity', 'rollback_identity', 'corpus_identity', 'images'):
            self.assertEqual(set(value[key]), {NEW})

    def test_shipped_policy_switches_channels(self):
        value = v.policy(HOST / 'deploy-policy.json')
        self.assertEqual(value['repositories'], [NEW])
        self.assertNotIn('repository', value)
        self.assertEqual(value['release_repository'], 'ghcr.io/dntmakers/trachuan-release')
        self.assertEqual(value['rollback_repository'], 'ghcr.io/dntmakers/trachuan-rollback')
        self.assertEqual(value['corpus_repository'], 'ghcr.io/dntmakers/trachuan-corpus-release')
        for repository in value['repositories']:
            owner = repository.split('/')[0]
            self.assertEqual(value['images'][repository],
                             {name: 'ghcr.io/' + owner + '/trachuan-' + name for name in ('server', 'web', 'queryembed', 'postgres')})
            for kind, workflow in WORKFLOWS.items():
                if kind == 'frameworks':
                    continue
                expression = value[kind + '_identity'][repository]
                self.assertIsNotNone(re.fullmatch(expression, 'https://github.com/' + repository + '/.github/workflows/'
                                                  + workflow + ('@refs/tags/v1.2.3' if workflow == 'images.yml' else '@refs/heads/main')))

    def test_template_images_env_names_the_last_repository(self):
        value = v.policy(HOST / 'deploy-policy.json')
        names = dict(value['images'][value['repositories'][-1]], **value['upstream'])
        pins = dict(line.split('=', 1) for line in (HOST / 'images.env').read_text().splitlines())
        self.assertEqual({key: reference.split('@')[0] for key, reference in pins.items()},
                         {name.upper() + '_IMAGE': repository for name, repository in names.items()})

    def test_one_repository_policy_is_accepted(self):
        value = dict(self.base, repositories=[NEW])
        for key in ('release_identity', 'build_identity', 'rollback_identity', 'corpus_identity', 'images'):
            value[key] = {NEW: self.base[key][NEW]}
        self.assertEqual(self.load(value)['repositories'], [NEW])

    def test_repository_list_itself_refuses_a_bad_count_duplicate_or_name(self):
        for names in ([NEW], [OLD, NEW]):
            self.assertEqual(self.load(keyed_policy(self.base, names))['repositories'], names)
        for names in ([], [OLD, NEW, 'third/trachuan'], [OLD, OLD], [OLD, 'bad name'], ['bad name'], [OLD, 'a/b/c'], {OLD: 1}):
            with self.subTest(names=names), self.assertRaisesRegex(ValueError, 'repositor'):
                self.load(keyed_policy(self.base, names))

    def test_repository_list_and_keyed_tables_are_closed(self):
        identity = self.base['release_identity']
        changes = [dict(repositories=OLD), dict(repositories=[OLD, None]), dict(repositories=[OLD, [NEW]]),
                   dict(repository=OLD), dict(release_identity={OLD: identity[OLD]}),
                   dict(release_identity=dict(identity, extra=identity[OLD])), dict(release_identity=identity[OLD]),
                   dict(build_identity=dict(identity, **{NEW: '.*'})), dict(rollback_identity=dict(identity, **{NEW: '^(x$'})),
                   dict(build_identity=dict(identity, **{NEW: identity[NEW][:-1]})),
                   dict(corpus_identity=dict(self.base['corpus_identity'], **{NEW: self.base['corpus_identity'][NEW][1:]})),
                   dict(corpus_identity={OLD: self.base['corpus_identity'][OLD]}),
                   dict(images=self.base['images'][OLD]), dict(images={OLD: self.base['images'][OLD]}),
                   dict(images=dict(self.base['images'], **{NEW: self.base['images'][OLD]})),
                   dict(images=dict(self.base['images'], **{NEW: dict(self.base['images'][NEW], server='ghcr.io/x/trachuan-server')})),
                   dict(images=dict(self.base['images'], **{NEW: {'server': 'ghcr.io/dntmakers/trachuan-server'}}))]
        for change in changes:
            value = {key: item for key, item in dict(self.base, **change).items()}
            if 'repository' in change:
                value.pop('repositories')
            with self.subTest(change=change), self.assertRaises(ValueError):
                self.load(value)

    def test_frameworks_identity_is_keyed_and_differs_from_corpus_per_repository(self):
        block = rules()['frameworks']
        self.assertEqual(self.load(dict(self.base, frameworks=block))['frameworks'], block)
        for identity in (frameworks_identity(OLD), {OLD: frameworks_identity(OLD)},
                         dict(block['identity'], **{NEW: self.base['corpus_identity'][NEW]}),
                         dict(block['identity'], **{NEW: '.*'})):
            with self.subTest(identity=identity), self.assertRaises(ValueError):
                self.load(dict(self.base, frameworks=dict(block, identity=identity)))


class ReleaseRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.rules = rules()
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)
        self.old = json.loads((FIXTURES / 'manifest.json').read_bytes())

    def write(self, obj):
        (self.path / 'manifest.json').write_bytes(canonical(obj))
        (self.path / 'manifest.sigstore.json').write_bytes(b'{}')

    def test_shipped_policy_refuses_dropped_owner_release(self):
        rules = v.policy(HOST / 'deploy-policy.json')
        with self.assertRaisesRegex(ValueError, 'repository'):
            v.release(canonical(self.old), rules)

    def test_release_from_each_owner_is_verified_with_that_owners_identity(self):
        for repository in (OLD, NEW):
            obj = moved(self.old, repository)
            self.write(obj)
            cosign = FakeCosign(repository, 'images.yml', obj['ref'])
            with self.subTest(repository=repository):
                self.assertEqual(v.verify_manifest(self.path, self.rules, cosign)[1], obj)
                self.assertEqual(cosign.flags('--certificate-github-workflow-repository'), [repository])
                self.assertEqual(cosign.flags('--certificate-identity-regexp'), [self.rules['release_identity'][repository]])

    def test_release_claiming_one_owner_with_the_other_owners_certificate_is_refused(self):
        for claimed, signer in ((NEW, OLD), (OLD, NEW)):
            obj = moved(self.old, claimed)
            self.write(obj)
            with self.subTest(claimed=claimed), self.assertRaises(CommandError):
                v.verify_manifest(self.path, self.rules, FakeCosign(signer, 'images.yml', obj['ref']))

    def test_release_repository_outside_the_policy_is_refused_before_cosign(self):
        for repository in ('attacker/trachuan', 'dannyota/other', 'DANNYOTA/trachuan', None, ['dannyota/trachuan']):
            with self.subTest(repository=repository), self.assertRaises(ValueError):
                v.release(canonical(dict(self.old, repository=repository)), self.rules)

    def test_release_images_must_be_the_claimed_repositorys_images(self):
        mixed = moved(self.old, NEW)
        mixed['images']['web'] = self.old['images']['web']
        for obj in (dict(self.old, repository=NEW), dict(moved(self.old, NEW), repository=OLD), mixed):
            with self.subTest(obj=obj['images']['web']), self.assertRaises(ValueError):
                v.release(canonical(obj), self.rules)

    def test_build_attestation_binds_the_manifest_repository(self):
        obj = moved(self.old, NEW)
        image = obj['images']['server']
        cosign = FakeCosign(NEW, 'images.yml', obj['ref'])

        def runner(args, **kwargs):
            cosign(args)
            definition = (proof := statement(obj, args[-1]))['predicate']['buildDefinition']
            definition['externalParameters']['workflow']['repository'] = 'https://github.com/' + NEW
            definition['resolvedDependencies'][0]['uri'] = 'git+https://github.com/' + NEW + '@' + obj['ref']
            return canonical({'payload': base64.b64encode(canonical(proof)).decode()})

        v.Verifier(self.rules, runner, user_prefix=[]).attestation(obj, image)
        self.assertEqual(cosign.flags('--certificate-identity-regexp'), [self.rules['build_identity'][NEW]])
        self.assertEqual(cosign.flags('--certificate-github-workflow-repository'), [NEW])
        with self.assertRaisesRegex(ValueError, 'wrong attestation'):
            v.provenance(statement(obj, image), obj, image, self.rules)


class RollbackRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.rules = rules()
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)
        self.path.joinpath('manifest.json').write_bytes(
            canonical(dict(kind='trachuan.rollback', version=1, current='a' * 64, target='b' * 64, sequence=3)))
        self.path.joinpath('manifest.sigstore.json').write_bytes(b'{}')

    def test_rollback_verifies_against_either_entry_with_that_entrys_identity(self):
        for signer, attempts in ((OLD, [OLD]), (NEW, [OLD, NEW])):
            cosign = FakeCosign(signer, 'rollback.yml', 'refs/heads/main')
            with self.subTest(signer=signer):
                self.assertEqual(v.verify_manifest(self.path, self.rules, cosign, 'rollback')[1]['sequence'], 3)
                self.assertEqual(cosign.flags('--certificate-github-workflow-repository'), attempts)
                self.assertEqual(cosign.flags('--certificate-identity-regexp'),
                                 [self.rules['rollback_identity'][name] for name in attempts])
                self.assertEqual(set(cosign.flags('--certificate-github-workflow-ref')), {'refs/heads/main'})

    def test_rollback_is_refused_when_no_entry_identity_matches(self):
        for signer, workflow in (('attacker/trachuan', 'rollback.yml'), (OLD, 'images.yml'), (NEW, 'corpus-release.yml')):
            cosign = FakeCosign(signer, workflow, 'refs/heads/main')
            with self.subTest(signer=signer, workflow=workflow), self.assertRaises(CommandError):
                v.verify_manifest(self.path, self.rules, cosign, 'rollback')
            self.assertEqual(len(cosign.calls), 2)

    def test_only_a_cosign_refusal_moves_on_to_the_next_entry(self):
        for error in (ValueError('invalid bundle'), OSError('unreadable bundle'), KeyError('rollback_identity'),
                      CommandError('command failed'), CommandError('command output too large')):
            calls = []

            def fail(args, **kwargs):
                calls.append(args)
                raise error

            with self.subTest(error=error), self.assertRaises(type(error)):
                v.verify_manifest(self.path, self.rules, fail, 'rollback')
            self.assertEqual(len(calls), 1)

    def test_a_command_error_without_an_exit_code_stops_the_loop(self):
        calls = []

        def fail(args, **kwargs):
            calls.append(args)
            raise CommandError('command failed')

        with self.assertRaises(CommandError):
            v.verify_manifest(self.path, self.rules, fail, 'rollback')
        self.assertEqual(len(calls), 1)

    def test_cosign_killed_by_a_signal_stops_the_loop(self):
        calls = []

        def killed(args, **kwargs):
            calls.append(args)
            raise CommandError('command failed', returncode=-9)

        with self.assertRaises(CommandError):
            v.verify_manifest(self.path, self.rules, killed, 'rollback')
        self.assertEqual(len(calls), 1)


class CorpusRepositoryTest(unittest.TestCase):
    def setUp(self):
        self.rules = rules()
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)

    def write(self, obj):
        (self.path / 'manifest.json').write_bytes(canonical(obj))
        (self.path / 'manifest.sigstore.json').write_bytes(b'{}')

    def cases(self):
        return ((corpus, json.loads((FIXTURES / 'corpus-manifest.json').read_bytes()), 'corpus-release.yml'),
                (frameworks, json.loads((FIXTURES / 'frameworks-manifest.json').read_bytes()), 'frameworks-release.yml'))

    def test_corpus_and_frameworks_from_each_owner_are_accepted(self):
        for module, base, workflow in self.cases():
            for repository in (OLD, NEW):
                obj = dict(base, repository=repository)
                self.write(obj)
                cosign = FakeCosign(repository, workflow, 'refs/heads/main')
                with self.subTest(workflow=workflow, repository=repository):
                    self.assertEqual(module.verify(self.path, self.rules, cosign)[1], obj)
                    self.assertEqual(cosign.flags('--certificate-github-workflow-repository'), [repository])
                    table = self.rules['frameworks']['identity'] if module is frameworks else self.rules['corpus_identity']
                    self.assertEqual(cosign.flags('--certificate-identity-regexp'), [table[repository]])

    def test_corpus_and_frameworks_with_the_other_owners_certificate_are_refused(self):
        for module, base, workflow in self.cases():
            for claimed, signer in ((NEW, OLD), (OLD, NEW)):
                self.write(dict(base, repository=claimed))
                with self.subTest(workflow=workflow, claimed=claimed), self.assertRaises(CommandError):
                    module.verify(self.path, self.rules, FakeCosign(signer, workflow, 'refs/heads/main'))

    def test_corpus_and_frameworks_repository_outside_the_policy_is_refused(self):
        for module, base, workflow in self.cases():
            for repository in ('attacker/trachuan', 'dntmakers/other', None):
                with self.subTest(workflow=workflow, repository=repository), self.assertRaises(ValueError):
                    module.manifest(canonical(dict(base, repository=repository)), self.rules)

    def test_one_repository_policy_refuses_the_dropped_owner(self):
        narrowed = copy.deepcopy(self.rules)
        narrowed['repositories'] = [NEW]
        for module, base, workflow in self.cases():
            with self.subTest(workflow=workflow), self.assertRaises(ValueError):
                module.manifest(canonical(dict(base, repository=OLD)), narrowed)


if __name__ == '__main__':
    unittest.main()
