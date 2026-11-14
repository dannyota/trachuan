import copy
import importlib
import json
from pathlib import Path
import tempfile
import unittest

from corpus_test_support import digest, save_fixture, write_fixture
from manifest import canonical


class CorpusManifestTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_manifest')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.row, self.report = write_fixture(self.path)

    def validate(self):
        return self.module.validate_inputs('vn', '20990102_1', self.path, self.path / 'report.json')

    def test_canonical_manifest_has_exact_contract_and_no_clock(self):
        obj = self.module.create('a' * 40, 57, 'vn', '20990102_1', self.path, self.path / 'report.json')
        expected = dict(kind='trachuan.corpus-release', version=1, repository='dannyota/trachuan',
                        commit='a' * 40, country='vn', rel='20990102_1', database='corpus_vn_20990102_1',
                        sequence=57, previous='20990101_1',
                        dump=dict(object='vn/20990102_1/corpus.dump',
                                  sha256=digest((self.path / 'corpus.dump').read_bytes()),
                                  size=(self.path / 'corpus.dump').stat().st_size),
                        report=dict(sha256=digest((self.path / 'report.json').read_bytes()), quality_pass=True),
                        build_artifact_sha256=self.report['build_artifact_sha256'], schema_version=6,
                        extensions=self.row['extensions'],
                        recipes=dict(dense=self.row['dense_recipe'], chunk=self.row['chunk_recipe'],
                                     lexical=self.row['lexical_recipe'], retrieval=self.row['retrieval_recipe']))
        self.assertEqual(obj, canonical(expected))
        self.assertEqual(self.module.read_manifest(obj), expected)
        with self.assertRaises(ValueError):
            self.module.read_manifest(obj + b'\n')

    def test_rejects_report_and_release_contract_violations(self):
        mutations = [
            lambda r, e: e.update(dump_sha256='0' * 64),
            lambda r, e: e.update(dump_sha256='BAD'),
            lambda r, e: e.update(build_artifact_sha256='0' * 64),
            lambda r, e: e.update(quality_pass=False),
            lambda r, e: e.update(quality_pass=1),
            lambda r, e: e.update(release_eligible=True),
            lambda r, e: e.update(status='incomplete'),
            lambda r, e: e.update(version=2),
            lambda r, e: e.update(country='us'),
            lambda r, e: e.update(rel='20990103_1'),
            lambda r, e: e.update(cap_override='none'),
            lambda r, e: e.update(article_cap_override=0),
            lambda r, e: e.update(historical_policy='agent'),
            lambda r, e: e.update(non_current_tail=0),
            lambda r, e: e.update(not_yet_eligible=True),
            lambda r, e: e.update(not_run={'filtered': 'research_override'}),
            lambda r, e: e.update(previous_diff=None),
            lambda r, e: e.update(previous_diff={'reason': 'no previous report'}),
            lambda r, e: e['previous_diff']['previous'].update(country='us'),
            lambda r, e: e['previous_diff']['previous'].update(report_sha256='bad'),
            lambda r, e: e['non_regression'][0].update(status='fail'),
            lambda r, e: e['cohorts']['old_product']['non_regression'][0].update(status='not_run'),
            lambda r, e: e['cohorts']['development']['non_regression'].append(dict(gating=True, status='unavailable')),
            lambda r, e: e['recipes'].update(retrieval='0' * 64),
            lambda r, e: e['recipes'].update(unexpected='0' * 64),
            lambda r, e: r['retrieval_recipe'].update(version=4),
            lambda r, e: r['dense_recipe'].update(dimensions=256),
            lambda r, e: r['dense_recipe'].update(model='another model'),
            lambda r, e: r['extensions'].update(pg_prewarm='1.2'),
            lambda r, e: r['extensions'].pop('vector'),
            lambda r, e: r['run'].update(complete=False),
            lambda r, e: r['run'].update(build_artifact_sha256='0' * 64),
            lambda r, e: r.update(country='us'),
        ]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                row, report = copy.deepcopy(self.row), copy.deepcopy(self.report)
                mutate(row, report)
                save_fixture(self.path, row, report)
                with self.assertRaises(ValueError):
                    self.validate()

    def test_rejects_changed_dump_and_duplicate_manifest_line(self):
        (self.path / 'corpus.dump').write_bytes(b'another dump')
        with self.assertRaises(ValueError):
            self.validate()
        write_fixture(self.path)
        path = self.path / 'MANIFEST.sha256'
        path.write_bytes(path.read_bytes() * 2)
        self.report['build_artifact_sha256'] = digest(path.read_bytes())
        self.row['run']['build_artifact_sha256'] = self.report['build_artifact_sha256']
        save_fixture(self.path, self.row, self.report)
        with self.assertRaises(ValueError):
            self.validate()

    def test_rejects_duplicate_json_keys(self):
        path = self.path / 'report.json'
        path.write_text(path.read_text().replace('"quality_pass": true', '"quality_pass": false,"quality_pass": true'))
        with self.assertRaises(ValueError):
            self.validate()

    def test_recipe_hash_inputs_are_exact_bytes_not_sorted_json(self):
        self.row['recipe_hash_inputs']['chunk'] = '{ "version": 1 }'
        self.report['recipes']['chunk'] = digest(b'{ "version": 1 }')
        save_fixture(self.path, self.row, self.report)
        self.validate()

    def test_requires_all_emitted_gates_in_top_level_and_populated_cohorts(self):
        for location in ('top', 'old_product', 'development'):
            for change in ('missing', 'empty', 'omit', 'downgrade', 'not_run', 'duplicate', 'wrong_mode', 'wrong_limit'):
                with self.subTest(location=location, change=change):
                    row, report = copy.deepcopy(self.row), copy.deepcopy(self.report)
                    target = report if location == 'top' else report['cohorts'][location]
                    records = target['non_regression']
                    if change == 'missing':
                        target.pop('non_regression')
                    elif change == 'empty':
                        target['non_regression'] = []
                    elif change == 'omit':
                        records.pop(0)
                    elif change == 'downgrade':
                        records[0]['gating'] = False
                    elif change == 'not_run':
                        records[0]['status'] = 'not_run'
                    elif change == 'duplicate':
                        records.append(records[0].copy())
                    elif change == 'wrong_mode':
                        records[0]['mode'] = 'other'
                    else:
                        records[0]['limit'] = 8
                    save_fixture(self.path, row, report)
                    with self.assertRaises(ValueError):
                        self.validate()
        for name in self.report['cohorts']:
            with self.subTest(missing_cohort=name):
                report = copy.deepcopy(self.report)
                report['cohorts'].pop(name)
                save_fixture(self.path, self.row, report)
                with self.assertRaises(ValueError):
                    self.validate()

    def test_empty_development_does_not_require_comparisons(self):
        self.report['cohorts']['development']['case_count'] = 0
        for record in self.report['cohorts']['development']['non_regression']:
            record['status'] = 'not_run'
        save_fixture(self.path, self.row, self.report)
        self.validate()


if __name__ == '__main__':
    unittest.main()
