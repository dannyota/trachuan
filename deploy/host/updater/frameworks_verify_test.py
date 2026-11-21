import json
from pathlib import Path
import tempfile
import unittest

from updater.corpus_verify_test import FIXTURE as LAW_FIXTURE
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_corpus_verify import manifest as law_manifest
from updater import deploy_frameworks_verify as v
from updater.deploy_host import Host
from updater.deploy_io import canonical, CommandError
from updater.deploy_store import Paths
from updater.deploy_verify import policy
from updater.verification_test import FIXTURE_POLICY


FIXTURE = Path(__file__).with_name('fixtures') / 'frameworks-manifest.json'
POLICY = Path(__file__).parents[1] / 'deploy-policy.json'
NESTED = b'{"kind":' + b'[' * 500000 + b']' * 500000 + b'}'
BLOCK = dict(repository='ghcr.io/dannyota/trachuan-corpus-release',
             identity={name: '^https://github\\.com/' + name + '/\\.github/workflows/frameworks-release\\.yml@refs/heads/main$'
                       for name in FIXTURE_POLICY['repositories']},
             namespace='syntheticnamespace', bucket='synthetic-frameworks', min_release_floor=40)


def manifest():
    return json.loads(FIXTURE.read_bytes())


def rules():
    return dict(json.loads(json.dumps(FIXTURE_POLICY)), frameworks=dict(BLOCK))


def description(m):
    return dict(schema_versions=[1], retrieval_recipe_versions=[m['recipes']['retrieval']['version']],
                lexical_recipe_versions=[m['recipes']['lexical']['version']],
                query_recipe_hash=m['recipes']['dense']['query_hash'])


class FrameworksPolicyTest(unittest.TestCase):
    def load(self, value):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'policy.json'
            path.write_bytes(canonical(value))
            return policy(path)

    def test_block_is_optional_and_validated_as_closed_shape(self):
        self.assertNotIn('frameworks', self.load(json.loads(POLICY.read_bytes())))
        base = json.loads(json.dumps(FIXTURE_POLICY))
        self.assertEqual(self.load(dict(base, frameworks=BLOCK))['frameworks'], BLOCK)
        changes = [dict(extra=1), dict(identity='.*'), dict(identity=base['corpus_identity']),
                   dict(identity='^(unclosed$'), dict(repository='docker.io/evil/release'),
                   dict(namespace='../other'), dict(bucket='other/bucket'), dict(min_release_floor=0),
                   dict(min_release_floor=True), dict(min_release_floor='40')]
        for change in changes:
            with self.subTest(change=change), self.assertRaises(ValueError):
                self.load(dict(base, frameworks=dict(BLOCK, **change)))
        for value in (None, [], {}, {key: BLOCK[key] for key in list(BLOCK)[1:]}):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.load(dict(base, frameworks=value))


class FrameworksVerificationTest(unittest.TestCase):
    def setUp(self):
        self.rules = rules()
        self.m = manifest()

    def test_manifest_field_set_is_closed_and_bound_to_kind_and_names(self):
        self.assertEqual(v.manifest(FIXTURE.read_bytes(), self.rules), self.m)
        self.assertIsNone(v.manifest(canonical(dict(self.m, previous=None)), self.rules)['previous'])
        self.assertEqual(v.manifest(canonical(dict(self.m, rel='20990102_12', database='frameworks_20990102_12',
            dump=dict(self.m['dump'], object='frameworks/20990102_12/frameworks.dump'))), self.rules)['rel'], '20990102_12')
        recipes = self.m['recipes']
        changes = [dict(corpus_kind='law'), dict(corpus_kind=None), dict(kind='trachuan.image-release'),
                   dict(version=True), dict(repository='attacker/repo'), dict(commit='short'),
                   dict(rel='2099010'), dict(rel='20990102_123'), dict(rel='20990102-1'),
                   dict(database='corpus_vn_20990102'), dict(database='frameworks_20990103'),
                   dict(previous='bad'), dict(previous=''), dict(sequence=0), dict(sequence=True),
                   dict(schema_version=0), dict(dump=dict(self.m['dump'], object='vn/20990102/corpus.dump')),
                   dict(dump=dict(self.m['dump'], sha256='bad')), dict(dump=dict(self.m['dump'], size=0)),
                   dict(dump=dict(self.m['dump'], extra=1)), dict(report=dict(self.m['report'], quality_pass=False)),
                   dict(report=dict(self.m['report'], extra=1)), dict(build_artifact_sha256='bad'),
                   dict(extensions=dict(vector='0.8.1', plpython3u='1.0')), dict(extensions=dict(vector='x', ltree='1.3')),
                   dict(counts=dict(versions=3)), dict(counts=dict(versions=3, controls=1612, text=1)),
                   dict(counts=dict(versions=0, controls=1612)), dict(counts=dict(versions=3, controls=True)),
                   dict(recipes={k: recipes[k] for k in ('dense', 'chunk', 'lexical')}),
                   dict(recipes=dict(recipes, extra={'version': 1})), dict(recipes=dict(recipes, chunk={})),
                   dict(recipes=dict(recipes, dense=dict(recipes['dense'], query_hash='bad'))),
                   dict(recipes=dict(recipes, lexical=dict(recipes['lexical'], version='1'))),
                   dict(recipes=dict(recipes, retrieval={k: x for k, x in recipes['retrieval'].items() if k != 'version'}))]
        for change in changes:
            with self.subTest(change=change), self.assertRaises(ValueError):
                v.manifest(canonical(dict(self.m, **change)), self.rules)
        for key in ('counts', 'previous', 'corpus_kind'):
            with self.subTest(missing=key), self.assertRaises(ValueError):
                v.manifest(canonical({k: x for k, x in self.m.items() if k != key}), self.rules)
        with self.assertRaises(ValueError):
            v.manifest(canonical(dict(self.m, country='vn')), self.rules)
        with self.assertRaises(ValueError):
            v.manifest(FIXTURE.read_bytes() + b'\n', self.rules)

    def test_deeply_nested_or_undecodable_manifest_is_a_verification_refusal(self):
        for data in (NESTED, b'\xff\xfe', b'{"kind": NaN', b''):
            for verifier, message in ((v.manifest, 'invalid frameworks manifest JSON'),
                                      (law_manifest, 'invalid corpus manifest JSON')):
                with self.subTest(data=data[:12], verifier=message), self.assertRaisesRegex(ValueError, '^' + message + '$'):
                    verifier(data, self.rules)

    def test_law_and_frameworks_verifiers_refuse_each_others_manifests(self):
        with self.assertRaises(ValueError):
            law_manifest(FIXTURE.read_bytes(), self.rules)
        with self.assertRaises(ValueError):
            v.manifest(LAW_FIXTURE.read_bytes(), self.rules)
        law = json.loads(LAW_FIXTURE.read_bytes())
        with self.assertRaises(ValueError):
            law_manifest(canonical(dict(law, corpus_kind='law')), self.rules)
        with self.assertRaises(ValueError):
            law_manifest(canonical(dict(law, corpus_kind='frameworks')), self.rules)

    def test_signature_uses_frameworks_identity_main_ref_and_commit(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            path.joinpath('manifest.json').write_bytes(FIXTURE.read_bytes())
            path.joinpath('manifest.sigstore.json').write_bytes(b'{}')
            calls = []
            digest, obj = v.verify(path, self.rules, lambda args: calls.append(args))
            self.assertEqual(obj, self.m)
            args = calls[0]
            for flag, value in [('--certificate-identity-regexp', BLOCK['identity'][self.m['repository']]),
                                ('--certificate-github-workflow-repository', self.m['repository']),
                                ('--certificate-github-workflow-ref', 'refs/heads/main'),
                                ('--certificate-github-workflow-sha', self.m['commit']),
                                ('--certificate-oidc-issuer', self.rules['issuer'])]:
                self.assertEqual(args[args.index(flag) + 1], value)
            self.assertNotIn(self.rules['corpus_identity'][self.m['repository']], args)
            def reject(args):
                raise CommandError('wrong identity')
            with self.assertRaises(CommandError):
                v.verify(path, self.rules, reject)

    def test_compatibility_parses_closed_report_and_waits_on_any_difference(self):
        report = description(self.m)
        self.assertTrue(v.compatible(self.m, report))
        for change in (dict(schema_versions=[2]), dict(retrieval_recipe_versions=[1, 3]),
                       dict(lexical_recipe_versions=[]), dict(query_recipe_hash='f' * 64)):
            with self.subTest(change=change):
                self.assertFalse(v.compatible(self.m, dict(report, **change)))
        for bad in (dict(report, extra=1), {k: x for k, x in report.items() if k != 'query_recipe_hash'},
                    dict(report, schema_versions=[True]), dict(report, schema_versions=1),
                    dict(report, lexical_recipe_versions=['1']), dict(report, query_recipe_hash='bad'), []):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                v.compatible(self.m, bad)

    def test_release_check_runs_through_law_podman_invocation_with_kind(self):
        with tempfile.TemporaryDirectory() as directory:
            calls = []
            def runner(args, **kwargs):
                calls.append(args)
                return canonical(description(self.m))
            host = Host(Paths(Path(directory)), runner, user_prefix=[])
            self.assertEqual(Database(host, self.rules, FRAMEWORKS).describe('server-image'), description(self.m))
            Database(host, self.rules, LAW).describe('server-image')
            frameworks, law = calls
            self.assertEqual(frameworks, law + ['--kind', 'frameworks'])
            self.assertEqual(law[-2:], ['server-image', 'release-check'])


if __name__ == '__main__':
    unittest.main()
