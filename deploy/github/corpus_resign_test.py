import copy
import io
import json
from contextlib import redirect_stdout
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import corpus_production
from corpus_manifest import create, manifest_fields
from corpus_test_support import add_heading_build, digest, save_older_shape, write_evidence, write_fixture

PREVIOUS, REL, CUTOFF = '20990101_1', '20990102_1', '20990102_2'
DECISION = dict(eval_report_sha256='e' * 64, previous_report_sha256='b' * 64)
EXEMPT = {'commit', 'sequence', 'heading_review'}
FULL_MANIFEST_FIELDS = {'kind', 'version', 'repository', 'commit', 'country', 'rel', 'database', 'sequence',
                        'previous', 'dump', 'report', 'build_artifact_sha256', 'schema_version', 'extensions',
                        'recipes', 'heading_evidence', 'heading_review', 'evidence'}


def manifest(rel=REL, sequence=11, report='b' * 64, **fields):
    return dict(country='vn', rel=rel, sequence=sequence, commit='a' * 40,
                report=dict(sha256=report, quality_pass=True), **fields)


def report_for(rel=PREVIOUS, digest='b' * 64):
    return dict(previous_diff=dict(previous=dict(country='vn', rel=rel, report_sha256=digest)))


def leaf_paths(value, path=()):
    if isinstance(value, dict) and value:
        for key, child in value.items():
            yield from leaf_paths(child, path + (key,))
    else:
        yield path


def with_leaf(value, path, leaf):
    changed = copy.deepcopy(value)
    target = changed
    for key in path[:-1]:
        target = target[key]
    target[path[-1]] = leaf
    return changed


class SignedFieldsTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.row, self.report = write_fixture(self.path)
        add_heading_build(self.path, self.row, self.report)
        reviews = self.path / 'reviews.yaml'
        reviews.write_text(json.dumps([dict(release=REL, projection=1, scan_sha256='c' * 64, exclusions_sha256='d' * 64,
                                            reviewer='coordinator', date='2099-01-02')]))
        self.full = json.loads(create('a' * 40, 7, 'vn', REL, self.path, self.path / 'report.json',
                                      heading_reviews_path=reviews))

    def signable(self, rebuilt):
        with patch('corpus_production.production', return_value=self.full), \
                patch('corpus_production.existing_release', return_value=self.full):
            return corpus_production.require_signable(rebuilt, report_for(), DECISION, self.path)

    def test_exempt_set_is_exactly_the_three_fields(self):
        self.assertEqual(corpus_production.RESIGN_MAY_CHANGE, EXEMPT)

    def test_full_manifest_holds_every_field(self):
        self.assertEqual(set(self.full), FULL_MANIFEST_FIELDS)

    def test_every_value_outside_the_exempt_fields_is_held(self):
        for path in leaf_paths(self.full):
            with self.subTest(path=path):
                rebuilt = with_leaf(self.full, path, 'changed')
                if path[0] in EXEMPT:
                    self.assertIs(self.signable(rebuilt), self.full)
                    continue
                with self.assertRaises(ValueError) as refused:
                    self.signable(rebuilt)
                self.assertEqual(str(refused.exception), 'signing again would change signed fields: ' + path[0])

    def test_every_field_outside_the_exempt_set_must_stay_present(self):
        for field in self.full:
            with self.subTest(field=field):
                rebuilt = {key: value for key, value in self.full.items() if key != field}
                self.assertEqual(corpus_production.changed_fields(self.full, rebuilt),
                                 [] if field in EXEMPT else [field])

    def test_manifest_fields_match_the_golden_contract(self):
        files = {name: digest((self.path / name).read_bytes()) for name in ('corpus.dump', 'MANIFEST.sha256', 'report.json')}
        expected = dict(kind='trachuan.corpus-release', version=1, repository='dannyota/trachuan', commit='f' * 40,
                        country='vn', rel='20990102_1', database='corpus_vn_20990102_1', sequence=9,
                        previous='20990101_1',
                        dump=dict(object='vn/20990102_1/corpus.dump', sha256=files['corpus.dump'], size=22),
                        report=dict(sha256=files['report.json'], quality_pass=True),
                        build_artifact_sha256=files['MANIFEST.sha256'], schema_version=8,
                        extensions=dict(vector='0.8.1', ltree='1.3'),
                        recipes=dict(dense=self.row['dense_recipe'], chunk=self.row['chunk_recipe'],
                                     lexical=self.row['lexical_recipe'], retrieval=self.row['retrieval_recipe']),
                        heading_evidence=dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64),
                        evidence=dict(findings_sha256=digest((self.path / 'findings.json').read_bytes()),
                                      identity_sha256=digest((self.path / 'identity.jsonl').read_bytes()),
                                      changes_sha256=digest((self.path / 'changes.jsonl').read_bytes())))
        actual = manifest_fields('f' * 40, 9, 'vn', REL, self.path, self.path / 'report.json')
        self.assertEqual(actual, expected, 'a derived manifest field changed: signed releases keep their old value, '
                                           'so run resign-check for the production and rollback releases first')


class SigningTraceTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)

    def signable(self, rebuilt, current, existing=None, run=None, report=None):
        with patch('corpus_production.production', return_value=current), \
                patch('corpus_production.existing_release', return_value=existing):
            return corpus_production.require_signable(rebuilt, report or report_for(),
                                                      DECISION if run is None else run, self.path)

    def production(self, rel=PREVIOUS, sequence=9, digest='b' * 64):
        return manifest(rel=rel, sequence=sequence, report=digest)

    def test_first_signing_with_a_decision_needs_only_the_comparison(self):
        self.assertEqual(self.signable(manifest(), self.production())['rel'], PREVIOUS)
        with self.assertRaisesRegex(ValueError, 'not compared'):
            self.signable(manifest(), self.production(rel='20990101_2'))

    def test_retry_of_a_failed_activation_keeps_signed_fields(self):
        current = self.production(rel=REL, sequence=10, digest='c' * 64)
        signed = manifest(sequence=10, commit_note='kept')
        self.assertIs(self.signable(manifest(commit_note='kept'), current, existing=signed), current)
        with self.assertRaisesRegex(ValueError, 'commit_note'):
            self.signable(manifest(commit_note='changed'), current, existing=signed)

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': CUTOFF})
    def test_rollback_of_an_older_release_without_a_decision_is_allowed(self):
        current = self.production(rel='20990103_1', sequence=12, digest='c' * 64)
        self.assertIs(self.signable(manifest(), current, existing=manifest(sequence=10), run={}), current)

    def test_push_without_production_move_keeps_signed_fields_and_the_comparison(self):
        signed = manifest(sequence=10, previous=PREVIOUS)
        rebuilt = manifest(previous=PREVIOUS)
        self.assertEqual(self.signable(rebuilt, self.production(), existing=signed)['rel'], PREVIOUS)
        with self.assertRaisesRegex(ValueError, 'not compared'):
            self.signable(rebuilt, self.production(rel='20990101_2'), existing=signed)
        with self.assertRaisesRegex(ValueError, 'previous'):
            self.signable(manifest(previous='20990101_2'), self.production(), existing=signed)

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL})
    def test_cutoff_release_signs_first_and_again_without_a_decision(self):
        self.assertEqual(self.signable(manifest(), self.production(), run={})['rel'], PREVIOUS)
        current = self.production(rel=REL, sequence=10, digest='c' * 64)
        self.assertIs(self.signable(manifest(), current, existing=manifest(sequence=10), run={}), current)

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': CUTOFF})
    def test_back_dated_new_release_is_refused_even_when_the_comparison_passes(self):
        with self.assertRaisesRegex(ValueError, 'signed manifest'):
            self.signable(manifest(), self.production(), run={})

    def test_every_held_field_must_keep_its_value(self):
        signed = manifest(sequence=10, previous=PREVIOUS, dump=dict(size=1), schema_version=8)
        current = self.production(rel=REL, sequence=10)
        changes = dict(previous='20990101_2', dump=dict(size=2), schema_version=9, report=dict(sha256='0' * 64))
        for field, value in changes.items():
            with self.subTest(field=field), self.assertRaisesRegex(ValueError, field):
                self.signable(copy.deepcopy(signed) | {field: value}, current, existing=signed)
        missing = copy.deepcopy(signed)
        missing.pop('schema_version')
        with self.assertRaisesRegex(ValueError, 'schema_version'):
            self.signable(missing, current, existing=signed)

    def test_signed_evidence_cannot_change_or_disappear_on_a_later_signing(self):
        evidence = dict(findings_sha256='a' * 64, identity_sha256='b' * 64, changes_sha256='c' * 64)
        signed = manifest(sequence=10, evidence=evidence)
        current = self.production(rel=REL, sequence=10)
        self.assertIs(self.signable(manifest(evidence=dict(evidence)), current, existing=signed), current)
        for rebuilt in (manifest(evidence=evidence | dict(identity_sha256='d' * 64)), manifest()):
            with self.subTest(rebuilt=rebuilt), self.assertRaisesRegex(ValueError, 'changed fields: evidence|signed fields: evidence'):
                self.signable(rebuilt, current, existing=signed)

    def test_values_compare_by_their_json_form(self):
        signed = manifest(sequence=10, dump=dict(size=1))
        with self.assertRaisesRegex(ValueError, 'dump'):
            self.signable(manifest(dump=dict(size=1.0)), self.production(rel=REL, sequence=10), existing=signed)

    def test_commit_sequence_and_heading_review_may_change_and_new_fields_are_allowed(self):
        signed = manifest(sequence=10, heading_review=dict(reviewer='old'))
        rebuilt = manifest(sequence=11, heading_review=dict(reviewer='new'), heading_evidence=dict(projection=1))
        rebuilt['commit'] = 'b' * 40
        current = self.production(rel=REL, sequence=10)
        self.assertIs(self.signable(rebuilt, current, existing=signed), current)
        without_review = manifest(sequence=11)
        self.assertIs(self.signable(without_review, current, existing=signed), current)


class ResignCheckTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.row, self.report = write_fixture(self.path)
        cutoff = patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': CUTOFF})
        cutoff.start()
        self.addCleanup(cutoff.stop)
        save_older_shape(self.path, self.row)
        self.signed = json.loads(create('c' * 40, 7, 'vn', REL, self.path, self.path / 'report.json'))

    def run_check(self):
        output = io.StringIO()
        argv = ['corpus_production.py', 'resign-check', 'vn', REL, str(self.path)]
        with patch('sys.argv', argv), redirect_stdout(output), \
                patch('corpus_production.existing_release', return_value=self.signed) as existing:
            try:
                corpus_production.main()
                code = 0
            except SystemExit as stop:
                code = stop.code
        self.assertEqual(existing.call_args.args[:2], ('vn', REL))
        return code, output.getvalue()

    def test_matching_upload_directory_reports_no_difference(self):
        code, output = self.run_check()
        self.assertEqual(code, 0)
        self.assertEqual(output, 'no signed field would change\n')

    def test_each_differing_field_is_printed_with_both_values(self):
        self.signed['previous'] = '20990101_2'
        self.signed['dump']['size'] += 1
        self.signed['retired'] = True
        code, output = self.run_check()
        self.assertEqual(code, 3)
        lines = output.splitlines()
        self.assertEqual([line.split(':', 1)[0] for line in lines], ['dump', 'previous', 'retired'])
        self.assertIn('previous: signed "20990101_2" rebuilt "20990101_1"', lines)
        self.assertIn('retired: signed true rebuilt absent', lines)

    def test_fields_signing_may_change_are_not_reported(self):
        self.signed.update(commit='d' * 40, sequence=1, heading_review=dict(reviewer='old'))
        self.assertEqual(self.run_check(), (0, 'no signed field would change\n'))

    def test_new_local_field_is_not_a_difference(self):
        add_heading_build(self.path, self.row, self.report)
        save_older_shape(self.path, self.row)
        self.signed = json.loads(create('c' * 40, 7, 'vn', REL, self.path, self.path / 'report.json'))
        self.signed.pop('heading_evidence')
        self.assertEqual(self.run_check()[0], 0)

    def test_heading_reviews_file_is_not_needed(self):
        with patch('corpus_manifest.heading_review', side_effect=AssertionError('read')):
            self.assertEqual(self.run_check()[0], 0)

    def test_missing_signed_manifest_refuses(self):
        self.signed = None
        with self.assertRaisesRegex(ValueError, 'no signed manifest'):
            self.run_check()

    def test_invalid_upload_directory_refuses(self):
        (self.path / 'corpus.dump').write_bytes(b'another dump')
        with self.assertRaisesRegex(ValueError, 'dump'):
            self.run_check()

    def test_rebuilt_manifest_is_validated_as_signing_validates_it(self):
        with patch('corpus_production.validate_manifest', side_effect=ValueError('invalid manifest')), \
                self.assertRaisesRegex(ValueError, 'invalid manifest'):
            self.run_check()

    def test_decision_upload_applies_the_gate_as_signing_would(self):
        findings = json.loads((self.path / 'findings.json').read_bytes())
        findings['detectors']['unmatched_operative_claim']['complete'] = False
        write_evidence(self.path, self.row, self.report, findings=findings)
        with self.assertRaisesRegex(ValueError, 'detector not complete'):
            self.run_check()

    def test_upload_whose_evidence_differs_from_the_signed_manifest_is_reported(self):
        write_evidence(self.path, self.row, self.report)
        self.signed = json.loads(create('c' * 40, 7, 'vn', REL, self.path, self.path / 'report.json'))
        self.assertEqual(self.run_check(), (0, 'no signed field would change\n'))
        findings = json.loads((self.path / 'findings.json').read_bytes())
        findings['detectors']['unmatched_operative_claim']['observed'] = 4
        write_evidence(self.path, self.row, self.report, findings=findings)
        code, output = self.run_check()
        self.assertEqual(code, 3)
        self.assertEqual([line.split(':', 1)[0] for line in output.splitlines()], ['evidence'])
        save_older_shape(self.path, self.row)
        code, output = self.run_check()
        self.assertEqual(code, 3)
        self.assertTrue(output.startswith('evidence: signed {') and output.endswith(' rebuilt absent\n'), output)


if __name__ == '__main__':
    unittest.main()
