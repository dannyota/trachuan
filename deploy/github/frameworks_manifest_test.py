from repositories_test_support import enable_dual_policy as setUpModule

import copy
import importlib
import json
from pathlib import Path
import sys
import tempfile
import unittest

from frameworks_test_support import COMMIT, PREVIOUS, REL, canonical, digest, manifest_object, object_bytes, state, write_objects

ROOT = Path(__file__).resolve().parents[2]
sys.path.append(str(ROOT / 'deploy/host'))
HOST = importlib.import_module('updater.deploy_frameworks_verify')
POLICY = importlib.import_module('updater.verification_test').FIXTURE_POLICY
IDENTITY = {name: '^https://github\\.com/' + name + '/\\.github/workflows/frameworks-release\\.yml@refs/heads/main$'
            for name in POLICY['repositories']}
RULES = dict(POLICY, frameworks=dict(repository='ghcr.io/dannyota/trachuan-corpus-release', identity=IDENTITY,
                                     namespace='syntheticnamespace', bucket='synthetic-frameworks', min_release_floor=40))


class FrameworksManifestTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_manifest')
        self.release = importlib.import_module('frameworks_release')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)

    def document(self, value=None, sequence=3, commit=COMMIT):
        value = value or state()
        facts = self.release.validate_release(REL, write_objects(self.path / 'objects', value))
        return self.module.document('dannyota/trachuan', commit, sequence, REL, facts)

    def test_document_has_the_exact_design_fields_and_values(self):
        value = state()
        data = self.document(value)
        self.assertEqual(json.loads(data), manifest_object(value))
        self.assertEqual(data, canonical(json.loads(data)))
        self.assertEqual(set(json.loads(data)), self.module.FIELDS)
        self.assertEqual(self.module.FIELDS, HOST.FIELDS)

    def test_previous_is_the_named_release(self):
        value = state()
        value['report']['previous'] = dict(rel=PREVIOUS, report_sha256='b' * 64)
        self.assertEqual(json.loads(self.document(value))['previous'], PREVIOUS)

    def test_host_verifier_accepts_the_document_and_binds_the_frameworks_identity(self):
        data = self.document()
        self.assertEqual(HOST.manifest(data, RULES)['rel'], REL)
        directory = self.path / 'artifact'
        directory.mkdir()
        (directory / 'manifest.json').write_bytes(data)
        calls = []
        digest_value, obj = HOST.verify(directory, RULES, calls.append)
        self.assertEqual(digest_value, digest(data))
        self.assertEqual(obj['commit'], COMMIT)
        command = calls[0]
        self.assertEqual(command[command.index('--certificate-identity-regexp') + 1], IDENTITY['dannyota/trachuan'])
        self.assertEqual(command[command.index('--certificate-github-workflow-ref') + 1], 'refs/heads/main')

    def test_host_fixture_is_readable_by_the_workflow_validator(self):
        data = (ROOT / 'deploy/host/updater/fixtures/frameworks-manifest.json').read_bytes().rstrip(b'\n')
        self.assertEqual(self.module.read_manifest(data)['rel'], REL)

    def test_read_manifest_refuses_what_the_host_refuses(self):
        good = json.loads(self.document())
        changes = [lambda m: m.update(extra=1), lambda m: m.pop('counts'), lambda m: m.update(corpus_kind='law'),
                   lambda m: m.update(kind='other'), lambda m: m.update(version=2),
                   lambda m: m.update(repository='evil/repo'), lambda m: m.update(commit='z'),
                   lambda m: m.update(rel='bad'), lambda m: m.update(database='frameworks_20990103'),
                   lambda m: m.update(previous='bad'), lambda m: m.update(sequence=0),
                   lambda m: m.update(sequence=True), lambda m: m.update(schema_version=0),
                   lambda m: m['dump'].update(object='vn/x/corpus.dump'), lambda m: m['dump'].update(sha256='x'),
                   lambda m: m['dump'].update(size=0), lambda m: m['dump'].update(extra=1),
                   lambda m: m['report'].update(quality_pass=False), lambda m: m['report'].update(sha256='x'),
                   lambda m: m.update(build_artifact_sha256='x'), lambda m: m['extensions'].pop('ltree'),
                   lambda m: m['extensions'].update(other='1.0'), lambda m: m['extensions'].update(vector='latest'),
                   lambda m: m['counts'].update(controls=0), lambda m: m['counts'].update(extra=1),
                   lambda m: m['recipes'].pop('chunk'), lambda m: m['recipes'].update(chunk={}),
                   lambda m: m['recipes']['dense'].pop('query_hash'),
                   lambda m: m['recipes']['lexical'].update(version=0),
                   lambda m: m['recipes']['retrieval'].pop('version')]
        for index, change in enumerate(changes):
            value = copy.deepcopy(good)
            change(value)
            with self.subTest(index=index):
                with self.assertRaises(ValueError):
                    self.module.read_manifest(canonical(value))
                with self.assertRaises(ValueError):
                    HOST.manifest(canonical(value), RULES)

    def test_read_manifest_requires_canonical_bytes(self):
        data = self.document()
        with self.assertRaises(ValueError):
            self.module.read_manifest(data + b'\n')
        with self.assertRaises(ValueError):
            self.module.read_manifest(json.dumps(json.loads(data)).encode())

    def test_law_manifest_is_refused(self):
        law = (ROOT / 'deploy/host/updater/fixtures/corpus-manifest.json').read_bytes().rstrip(b'\n')
        with self.assertRaises(ValueError):
            self.module.read_manifest(law)

    def test_document_refuses_a_row_the_host_would_refuse(self):
        changes = [lambda v: v['row'].update(version_count=0), lambda v: v['row'].update(control_count=0),
                   lambda v: v['row']['extensions'].pop('ltree'), lambda v: v['row'].update(schema_version=0),
                   lambda v: v['row']['dense_recipe'].pop('query_hash'),
                   lambda v: v['row']['retrieval_recipe'].pop('version'),
                   lambda v: v['row']['lexical_recipe'].update(note='text'),
                   lambda v: v['row']['dense_recipe']['query'].update(note='text'),
                   lambda v: v['row']['retrieval_recipe'].update(status='placeholder')]
        for index, change in enumerate(changes):
            value = state()
            change(value)
            with self.subTest(index=index), self.assertRaises(ValueError):
                self.document(value)
        for commit, sequence in (('short', 3), (COMMIT, 0)):
            with self.subTest(commit=commit), self.assertRaises(ValueError):
                self.document(commit=commit, sequence=sequence)

    def test_report_hash_is_of_the_exact_report_bytes(self):
        value = state()
        self.assertEqual(json.loads(self.document(value))['report']['sha256'], digest(object_bytes(value)['report.json']))


if __name__ == '__main__':
    unittest.main()
