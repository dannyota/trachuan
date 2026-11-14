import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.deploy_io import canonical, CommandError
from updater.deploy_verify import policy
from updater import deploy_corpus_verify as v
from updater import deploy_corpus_download as d
from updater.deploy_registry import layers
from updater.deploy_verify import release


FIXTURE = Path(__file__).with_name('fixtures') / 'corpus-manifest.json'


def manifest():
    return json.loads(FIXTURE.read_bytes())


def description(m):
    r = m['recipes']
    return dict(schema_versions=[4, 5, 6], dense_query_hash=r['dense']['query_hash'],
                running_query=dict(recipe_hash=r['dense']['query_hash'], recipe=r['dense']['query']),
                lexical_recipe=r['lexical'], retrieval_recipes=[r['retrieval']])


class CorpusVerificationTest(unittest.TestCase):
    def setUp(self):
        self.rules = policy(Path(__file__).parents[1] / 'deploy-policy.json')
        self.m = manifest()

    def test_manifest_fields_bind_dump_report_database_and_extensions(self):
        self.assertEqual(v.manifest(FIXTURE.read_bytes(), self.rules), self.m)
        changes = [dict(repository='attacker/repo'), dict(version=True), dict(sequence=0),
                   dict(database='auth'), dict(previous='invalid'), dict(country='us'),
                   dict(dump=dict(self.m['dump'], object='vn/other/corpus.dump')),
                   dict(dump=dict(self.m['dump'], sha256='bad')),
                   dict(dump=dict(self.m['dump'], size=True)),
                   dict(report=dict(self.m['report'], quality_pass=False)),
                   dict(report=dict(self.m['report'], dump_sha256='f'*64)),
                   dict(extensions=dict(vector='0.8.1', plpython3u='1.0'))]
        for change in changes:
            with self.subTest(change=change), self.assertRaises(ValueError):
                v.manifest(canonical(dict(self.m, **change)), self.rules)

    def test_signature_binds_protected_main_identity_and_workflow_sha(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            path.joinpath('manifest.json').write_bytes(FIXTURE.read_bytes())
            path.joinpath('manifest.sigstore.json').write_bytes(b'{}')
            calls = []
            v.verify(path, self.rules, lambda args: calls.append(args))
            args = calls[0]
            for flag, value in [('--certificate-identity-regexp', self.rules['corpus_identity']),
                                ('--certificate-github-workflow-ref', 'refs/heads/main'),
                                ('--certificate-github-workflow-sha', self.m['commit'])]:
                self.assertEqual(args[args.index(flag)+1], value)
            self.assertFalse(any('--ignore' in a or '--insecure' in a for a in args))
            for reason in ('wrong identity', 'branch signer', 'wrong workflow sha'):
                def reject(args):
                    raise CommandError(reason)
                with self.subTest(reason=reason), self.assertRaises(CommandError):
                    v.verify(path, self.rules, reject)

    def test_schema_refuses_and_recipe_difference_waits(self):
        report = description(self.m)
        self.assertTrue(v.compatible(self.m, report))
        for change in [dict(dense_query_hash='f'*64), dict(running_query={'recipe_hash':'f'*64}),
                       dict(lexical_recipe={}), dict(retrieval_recipes=[])]:
            self.assertFalse(v.compatible(self.m, dict(report, **change)))
        with self.assertRaises(ValueError):
            v.compatible(self.m, dict(report, schema_versions=[1]))

    def test_download_refuses_overflow_truncation_and_hash_mismatch(self):
        for data in (b'synthetic dump extra', b'short', b'x'*14):
            with self.subTest(data=data), tempfile.TemporaryDirectory() as directory:
                target = Path(directory)/'corpus.dump'
                with self.assertRaises(ValueError):
                    d.stream(io.BytesIO(data), target, self.m['dump'])
                self.assertFalse(target.exists())
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory)/'corpus.dump'
            d.stream(io.BytesIO(b'synthetic dump'), target, self.m['dump'])
            self.assertEqual(target.read_bytes(), b'synthetic dump')

    def test_instance_principal_download_uses_policy_location_and_no_retries(self):
        sdk = mock.Mock()
        sdk.object_storage.ObjectStorageClient.return_value.get_object.return_value.data.raw = io.BytesIO(b'synthetic dump')
        with tempfile.TemporaryDirectory() as directory:
            d.fetch(sdk, 'ap-singapore-1', 'syntheticnamespace', 'synthetic-bucket', self.m['dump'], Path(directory)/'corpus.dump')
        sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.assert_called_once()
        client = sdk.object_storage.ObjectStorageClient
        self.assertEqual(client.call_args.kwargs['config'], {'region':'ap-singapore-1'})
        self.assertEqual(client.return_value.get_object.call_args.args,
                         ('syntheticnamespace','synthetic-bucket',self.m['dump']['object']))

    def test_toc_rejects_code_and_unlisted_extensions_and_filters_extension_members(self):
        base = '1; 0 0 EXTENSION - vector \n2; 0 0 COMMENT - EXTENSION vector \n3; 0 0 EXTENSION - ltree \n4; 0 0 TABLE public document activator\n'
        selected = v.toc(base, self.rules['extensions'])
        self.assertNotIn('EXTENSION', selected)
        self.assertIn('TABLE public document', selected)
        for extra in ('5; 0 0 FUNCTION public evil() activator',
                      '5; 0 0 EVENT TRIGGER - evil activator',
                      '5; 0 0 EXTENSION - plpython3u',
                      '5; 0 0 PROCEDURE public evil() activator'):
            with self.subTest(extra=extra), self.assertRaises(ValueError):
                v.toc(base+extra+'\n', self.rules['extensions'])
        with self.assertRaises(ValueError):
            v.toc('malformed\n', self.rules['extensions'])
        with self.assertRaises(ValueError):
            v.toc(base+'5; 0 0 COMMENT - EXTENSION \n', self.rules['extensions'])

    def test_policy_rejects_unanchored_corpus_identity_and_extension_changes(self):
        for change in [dict(corpus_identity='.*'), dict(extensions=['vector','plpython3u']),
                       dict(release_namespace='../other'), dict(release_bucket='other/bucket')]:
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory)/'policy.json'
                path.write_bytes(canonical(dict(self.rules, **change)))
                with self.subTest(change=change), self.assertRaises(ValueError):
                    policy(path)

    def test_corpus_artifact_contains_only_manifest_and_signature(self):
        artifact = dict(artifactType='application/vnd.trachuan.corpus-release.v1', layers=[
            dict(digest='sha256:'+'a'*64, size=10,
                 annotations={'org.opencontainers.image.title': name})
            for name in ('manifest.json','manifest.sigstore.json')])
        self.assertEqual(set(layers(artifact, 'corpus-release')), {'manifest.json','manifest.sigstore.json'})
        artifact['layers'].append(dict(digest='sha256:'+'a'*64, size=10,
                                      annotations={'org.opencontainers.image.title':'corpus.dump'}))
        with self.assertRaises(ValueError):
            layers(artifact, 'corpus-release')

    def test_image_pair_is_refused_with_clear_result(self):
        image = json.loads(FIXTURE.with_name('manifest.json').read_bytes())
        image['corpus_release'] = dict(rel=self.m['rel'], manifest_hash='a'*64)
        with self.assertRaisesRegex(ValueError, 'paired corpus and image activation is not implemented'):
            release(canonical(image), self.rules)
