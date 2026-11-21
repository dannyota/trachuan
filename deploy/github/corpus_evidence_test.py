from repositories_test_support import enable_dual_policy as setUpModule

import copy
from itertools import combinations
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import corpus_evidence
from corpus_evidence import CHANGES, FINDINGS, IDENTITY, SIDECARS, require_predecessor
from corpus_manifest import create, release_files, validate_manifest, validated_inputs
from corpus_rules import DECISION_FIELDS
from corpus_test_support import digest, evidence_bindings, findings_fixture, jsonl, write_evidence, write_fixture

REL = '20990102_1'


class EvidenceGateTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.row, self.report = write_fixture(self.path)

    def upload(self):
        return validated_inputs('vn', REL, self.path, self.path / 'report.json')[3]

    def sign(self):
        return json.loads(create('dannyota/trachuan', 'a' * 40, 2, 'vn', REL, self.path, self.path / 'report.json'))

    def assert_both_refuse(self, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.upload()
        with self.assertRaisesRegex(ValueError, message):
            self.sign()

    def report_bytes(self):
        return (self.path / 'report.json').read_bytes()

    def base(self):
        return evidence_bindings(self.path, self.row, self.report_bytes())

    def findings(self, **changes):
        return findings_fixture(self.path, self.row, self.report_bytes(), self.report) | changes

    def save_run(self, run):
        row = copy.deepcopy(self.row)
        row['run'] = run
        (self.path / 'release.json').write_text(json.dumps(row))

    def test_complete_evidence_passes_and_signs_its_three_hashes(self):
        summary = self.upload()
        manifest = self.sign()
        expected = {field: self.row['run'][field] for field in ('findings_sha256', 'identity_sha256', 'changes_sha256')}
        self.assertEqual(manifest['evidence'], expected)
        self.assertEqual(summary['evidence'], expected)
        self.assertEqual(summary['open'], dict(review=0, info=0))
        self.assertEqual(summary['changed'], dict(new=0, gone=0))
        self.assertEqual(summary['predecessor'], dict(rel='20990101_1', findings_sha256='none', identity_sha256='none'))
        self.assertEqual(release_files(self.row), ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json') + SIDECARS)

    def test_each_missing_sidecar_refuses(self):
        for name in SIDECARS:
            with self.subTest(name=name):
                data = (self.path / name).read_bytes()
                (self.path / name).unlink()
                self.assert_both_refuse('missing: ' + name)
                (self.path / name).write_bytes(data)

    def test_tampered_sidecar_bytes_refuse(self):
        for name in SIDECARS:
            with self.subTest(name=name):
                path = self.path / name
                original = path.read_bytes()
                if name == FINDINGS:
                    tampered = original.replace(b'"schema"', b'"schema" ', 1)
                else:
                    header, rest = original.split(b'\n', 1)
                    tampered = jsonl([dict(reversed(json.loads(header).items()))]) + rest
                path.write_bytes(tampered)
                self.assert_both_refuse(name + ' differs from release.json')
                path.write_bytes(original)

    def test_hand_edited_release_hashes_refuse(self):
        run = self.row['run']
        for run_change in (dict(identity_sha256=run['changes_sha256'], changes_sha256=run['identity_sha256']),
                           dict(findings_sha256='0' * 64), dict(identity_sha256=run['identity_sha256'].upper()),
                           dict(changes_sha256='sha256:' + run['changes_sha256']), dict(findings_sha256=None)):
            with self.subTest(change=run_change):
                self.save_run(run | run_change)
                self.assert_both_refuse()

    def test_partial_decision_fields_refuse_even_at_the_cutoff(self):
        for size in range(1, len(DECISION_FIELDS)):
            for kept in combinations(DECISION_FIELDS, size):
                run = {key: value for key, value in self.row['run'].items() if key not in DECISION_FIELDS or key in kept}
                with self.subTest(kept=kept), patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL}):
                    self.save_run(run)
                    self.assert_both_refuse()

    def test_release_row_from_an_older_binary_refuses(self):
        run = {key: value for key, value in self.row['run'].items()
               if key not in ('findings_sha256', 'identity_sha256', 'changes_sha256')}
        self.save_run(run)
        self.assert_both_refuse('decision fields')

    def test_dropping_every_decision_field_above_the_cutoff_refuses(self):
        run = {key: value for key, value in self.row['run'].items() if key not in DECISION_FIELDS}
        self.save_run(run)
        self.assert_both_refuse('no selected report decision')

    def test_bindings_must_match_dump_build_report_and_previous(self):
        changes = {'rel': '20990102_2', 'dump_sha256': 'sha256:' + '0' * 64,
                   'build_artifact_sha256': 'sha256:' + '0' * 64, 'report_sha256': 'sha256:' + '0' * 64}
        for name in SIDECARS:
            for key, value in changes.items():
                with self.subTest(name=name, key=key):
                    base = self.base() | {key: value}
                    if name == FINDINGS:
                        findings = self.findings()
                        findings['bindings'] = findings['bindings'] | {key: value}
                        write_evidence(self.path, self.row, self.report, findings=findings)
                    elif name == IDENTITY:
                        write_evidence(self.path, self.row, self.report, identity=[dict(schema=1, bindings=base)])
                    else:
                        header = dict(schema=1, bindings=base | dict(predecessor_rel='20990101_1', predecessor_identity_sha256='none'), baseline=True)
                        write_evidence(self.path, self.row, self.report, changes=[header])
                    self.assert_both_refuse('differ')
        write_evidence(self.path, self.row, self.report)
        findings = self.findings()
        findings['bindings']['predecessor_rel'] = '20990101_2'
        write_evidence(self.path, self.row, self.report, findings=findings)
        self.assert_both_refuse('previous_diff')
        header = dict(schema=1, bindings=self.base() | dict(predecessor_rel='20990101_2', predecessor_identity_sha256='none'), baseline=True)
        write_evidence(self.path, self.row, self.report, changes=[header])
        self.assert_both_refuse('previous_diff')

    def test_detector_not_complete_refuses(self):
        findings = self.findings()
        findings['detectors']['unmatched_operative_claim']['complete'] = False
        write_evidence(self.path, self.row, self.report, findings=findings)
        self.assert_both_refuse('detector not complete')

    def finding(self, detector, status='open', carried=False):
        return dict(key='sha256:' + '1' * 64, detector=detector, subject=dict(type='document', fields=['vn:vbpl:1']),
                    discriminator='', fingerprint='sha256:' + '2' * 64, semantics=1, severity='review',
                    first_rel='20990101_1', status=status, carried=carried)

    def test_open_finding_of_a_blocking_or_unlisted_detector_refuses_fired_or_carried(self):
        detectors = self.findings()['detectors'] | {'blocker': dict(complete=True, severity='blocking', observed=1, not_observable=0, excluded=0)}
        for finding in (self.finding('blocker'), self.finding('blocker', carried=True), self.finding('retired_detector', carried=True)):
            with self.subTest(finding=finding):
                write_evidence(self.path, self.row, self.report, findings=self.findings(detectors=detectors, findings=[finding]))
                self.assert_both_refuse('blocking or unlisted')

    def test_accepted_blocking_and_open_review_and_info_findings_are_allowed_and_counted(self):
        detectors = self.findings()['detectors'] | {'blocker': dict(complete=True, severity='blocking', observed=1, not_observable=0, excluded=0)}
        findings = [self.finding('blocker', status='accepted'), self.finding('unmatched_operative_claim'),
                    self.finding('unmatched_operative_claim', carried=True), self.finding('partial_without_provision_effect')]
        write_evidence(self.path, self.row, self.report, findings=self.findings(detectors=detectors, findings=findings))
        self.assertEqual(self.upload()['open'], dict(review=2, info=1))
        self.sign()

    def test_new_and_gone_findings_against_the_previous_release_are_counted(self):
        write_evidence(self.path, self.row, self.report, findings=self.findings(new=['a', 'b'], gone=['c']))
        self.assertEqual(self.upload()['changed'], dict(new=2, gone=1))

    def test_invalid_findings_shapes_refuse(self):
        mutations = [lambda f: f.update(schema=1), lambda f: f.pop('unmatched'), lambda f: f.update(extra=True),
                     lambda f: f['bindings'].pop('detector_registry_sha256'),
                     lambda f: f['bindings'].update(predecessor_findings_sha256='0' * 64),
                     lambda f: f['detectors']['unmatched_operative_claim'].update(severity='fatal'),
                     lambda f: f['detectors']['unmatched_operative_claim'].update(observed=-1),
                     lambda f: f['detectors']['unmatched_operative_claim'].pop('excluded'),
                     lambda f: f.update(findings=[dict(detector='x', status='closed')]),
                     lambda f: f.update(gone={})]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                findings = self.findings()
                mutate(findings)
                write_evidence(self.path, self.row, self.report, findings=findings)
                self.assert_both_refuse()

    def test_json_lines_are_bounded_typed_and_ordered(self):
        header = dict(schema=1, bindings=self.base())
        document = dict(type='document', key='vn:vbpl:1', absent=True)
        relation = dict(type='relation', kind='cites')
        cases = {'line over 1 MiB': jsonl([header, document | dict(key='x' * corpus_evidence.LINE_LIMIT)]),
                 'not JSON': jsonl([header]) + b'{not json\n',
                 'blank line': jsonl([header]) + b'\n',
                 'no final newline': jsonl([header, document])[:-1],
                 'unknown type': jsonl([header, dict(type='moved')]),
                 'out of order': jsonl([header, relation, document]),
                 'empty': b'',
                 'other schema': jsonl([dict(schema=2, bindings=self.base())]),
                 'header with extra field': jsonl([header | dict(baseline=True)]),
                 'duplicate key': jsonl([header]) + b'{"type":"document","type":"relation"}\n'}
        for name, data in cases.items():
            with self.subTest(name=name):
                write_evidence(self.path, self.row, self.report, identity=data)
                self.assert_both_refuse()

    def test_changes_baseline_must_agree_with_its_predecessor(self):
        base = self.base()
        named = base | dict(predecessor_rel='20990101_1', predecessor_identity_sha256='sha256:' + 'a' * 64)
        none = base | dict(predecessor_rel='20990101_1', predecessor_identity_sha256='none')
        record = dict(type='document', key='vn:vbpl:1', change='added', cause='membership')
        for changes in ([dict(schema=1, bindings=none, baseline=False)], [dict(schema=1, bindings=named, baseline=True)],
                        [dict(schema=1, bindings=none, baseline=True), record],
                        [dict(schema=1, bindings=none, baseline=1)]):
            with self.subTest(changes=changes):
                write_evidence(self.path, self.row, self.report, changes=changes)
                self.assert_both_refuse()
        write_evidence(self.path, self.row, self.report, changes=[dict(schema=1, bindings=named, baseline=False), record])
        self.assertEqual(self.upload()['predecessor']['identity_sha256'], 'a' * 64)

    def test_size_limits_apply_before_reading(self):
        for name in SIDECARS:
            with self.subTest(name=name), patch.dict('corpus_evidence.LIMITS', {name: (self.path / name).stat().st_size - 1}):
                self.assert_both_refuse('size limit')

    def test_older_shape_never_reads_or_lists_sidecars(self):
        for name in SIDECARS:
            (self.path / name).unlink()
        row = copy.deepcopy(self.row)
        for field in DECISION_FIELDS:
            row['run'].pop(field)
        (self.path / 'release.json').write_text(json.dumps(row))
        with patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL}):
            self.assertIsNone(self.upload())
            self.assertNotIn('evidence', self.sign())
        self.assertEqual(release_files(row), ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json'))


class GoSummaryContractTest(unittest.TestCase):
    def test_go_golden_summaries_pass_the_line_validator_with_every_record_type(self):
        testdata = Path(__file__).resolve().parents[2] / 'apps/corpus/internal/stage/evidence/testdata'
        for name, types in ((IDENTITY, corpus_evidence.IDENTITY_TYPES), (CHANGES, corpus_evidence.CHANGE_TYPES)):
            with self.subTest(name=name):
                lines = (testdata / name).read_bytes().splitlines()
                seen = {json.loads(line)['type'] for line in lines[1:]}
                digest_value, header, records = corpus_evidence.scan_lines(testdata / name, types, lambda record: record)
                self.assertEqual(digest_value, digest((testdata / name).read_bytes()))
                self.assertEqual(records, len(lines) - 1)
                self.assertEqual(seen, set(types))
                self.assertEqual(header['schema'], corpus_evidence.SUMMARY_SCHEMA)


def relation(kind='cites', target='vn:vbpl:2', **tail):
    record = dict(type='relation', kind=kind, method='vbpl_diagram',
                  source=dict(key='vn:vbpl:1', number='1/2099/TT', in_release=True, anchor=None),
                  target=dict(key=target, number='2/2099/TT', in_release=True, anchor=None))
    return record | (tail or dict(count=1))


class LineOrderTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name) / 'summary.jsonl'
        self.header = dict(schema=1, bindings=dict(rel=REL))

    def scan(self, data, types=corpus_evidence.IDENTITY_TYPES):
        self.path.write_bytes(data)
        return corpus_evidence.scan_lines(self.path, types, lambda record: record)

    def refuses(self, data, types=corpus_evidence.IDENTITY_TYPES, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.scan(data, types)

    def test_keys_increase_strictly_within_each_record_type(self):
        doc = lambda key: dict(type='document', key=key, absent=True)
        prov = lambda key, anchor: dict(type='provision', key=key, anchor=anchor)
        accepted = [self.header, doc('vn:vbpl:1'), doc('vn:vbpl:2'), prov('vn:vbpl:1', 'dieu-2'), prov('vn:vbpl:1', 'dieu-2/khoan-1'),
                    prov('vn:vbpl:2', 'dieu-1'), relation('amends_supplements'), relation('cites', 'vn:vbpl:1'), relation('cites')]
        self.assertEqual(self.scan(jsonl(accepted))[2], len(accepted) - 1)
        for name, lines in {'documents out of order': [doc('vn:vbpl:2'), doc('vn:vbpl:1')],
                            'duplicate document': [doc('vn:vbpl:1'), doc('vn:vbpl:1')],
                            'provisions out of order': [prov('vn:vbpl:1', 'dieu-2'), prov('vn:vbpl:1', 'dieu-1')],
                            'duplicate provision': [prov('vn:vbpl:1', 'dieu-1'), prov('vn:vbpl:1', 'dieu-1')],
                            'provision documents out of order': [prov('vn:vbpl:2', 'dieu-1'), prov('vn:vbpl:1', 'dieu-9')],
                            'relations out of order': [relation('cites'), relation('amends_supplements')],
                            'duplicate relation': [relation('cites'), relation('cites', count=2)]}.items():
            with self.subTest(name=name):
                self.refuses(jsonl([self.header] + lines), message='order')

    def test_relation_order_is_read_from_its_bytes_before_change_or_count(self):
        removed = relation('cites', 'vn:vbpl:3', change='removed', count=1)
        added = relation('cites', 'vn:vbpl:2', change='added', count=1)
        self.assertEqual(self.scan(jsonl([self.header, added, removed]), corpus_evidence.CHANGE_TYPES)[2], 2)
        self.refuses(jsonl([self.header, removed, added]), corpus_evidence.CHANGE_TYPES, 'order')
        change_first = dict(type='relation', change='added') | {k: v for k, v in added.items() if k not in ('type', 'change')}
        self.refuses(jsonl([self.header, change_first]), corpus_evidence.CHANGE_TYPES, 'relation')

    def test_relation_with_no_fields_or_a_bad_count_refuses(self):
        for record in (dict(type='relation'), relation(count=0), relation(count='1'), relation(count=1, extra=True),
                       dict(type='document'), dict(type='provision', key='vn:vbpl:1')):
            with self.subTest(record=record):
                self.refuses(jsonl([self.header, record]))

    def test_lines_must_be_go_compact_json(self):
        doc = dict(type='document', key='vn:vbpl:1', absent=True)
        compact = jsonl([self.header, doc])
        self.assertEqual(self.scan(compact)[2], 1)
        for name, data in {'CRLF': compact.replace(b'\n', b'\r\n'),
                           'CRLF header only': compact.replace(b'\n', b'\r\n', 1),
                           'spaced record': jsonl([self.header]) + json.dumps(doc).encode() + b'\n',
                           'spaced header': json.dumps(self.header).encode() + b'\n' + jsonl([doc]),
                           'escaped non-ASCII': jsonl([self.header]) + json.dumps(doc | dict(key='vn:vbpl:Đ'), separators=(',', ':')).encode() + b'\n',
                           'raw line separator': jsonl([self.header, doc | dict(key='vn:vbpl:\u2028')])}.items():
            with self.subTest(name=name):
                self.refuses(data, message='compact')
        escaped = jsonl([self.header]) + b'{"type":"document","key":"vn:vbpl:\\u2028","absent":true}\n'
        self.assertEqual(self.scan(escaped)[2], 1)

    def test_upload_and_signing_apply_the_order(self):
        write_fixture(Path(self.path.parent))
        directory = Path(self.path.parent)
        row = json.loads((directory / 'release.json').read_bytes())
        report = json.loads((directory / 'report.json').read_bytes())
        base = evidence_bindings(directory, row, (directory / 'report.json').read_bytes())
        doc = lambda key: dict(type='document', key=key, absent=True)
        write_evidence(directory, row, report, identity=[dict(schema=1, bindings=base), doc('vn:vbpl:2'), doc('vn:vbpl:1')])
        with self.assertRaisesRegex(ValueError, 'order'):
            validated_inputs('vn', REL, directory, directory / 'report.json')
        with self.assertRaisesRegex(ValueError, 'order'):
            create('dannyota/trachuan', 'a' * 40, 2, 'vn', REL, directory, directory / 'report.json')


class PredecessorEvidenceTest(unittest.TestCase):
    def sidecars(self, findings='none', identity='none'):
        return dict(predecessor=dict(rel='20990101_1', findings_sha256=findings, identity_sha256=identity))

    def test_first_release_with_evidence_follows_a_manifest_without_it(self):
        require_predecessor(self.sidecars(), dict(rel='20990101_1'))
        for sidecars in (self.sidecars(findings='a' * 64), self.sidecars(identity='b' * 64)):
            with self.subTest(sidecars=sidecars), self.assertRaisesRegex(ValueError, 'previous signed manifest'):
                require_predecessor(sidecars, dict(rel='20990101_1'))

    def test_predecessor_hashes_must_equal_its_signed_evidence(self):
        manifest = dict(evidence=dict(findings_sha256='a' * 64, identity_sha256='b' * 64, changes_sha256='c' * 64))
        require_predecessor(self.sidecars('a' * 64, 'b' * 64), manifest)
        for sidecars in (self.sidecars(), self.sidecars('a' * 64, 'c' * 64), self.sidecars('b' * 64, 'b' * 64)):
            with self.subTest(sidecars=sidecars), self.assertRaisesRegex(ValueError, 'previous signed manifest'):
                require_predecessor(sidecars, manifest)


class ManifestEvidenceFieldTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        write_fixture(self.path)
        self.manifest = json.loads(create('dannyota/trachuan', 'a' * 40, 2, 'vn', REL, self.path, self.path / 'report.json'))

    def test_evidence_field_is_optional_and_closed(self):
        validate_manifest(self.manifest)
        without = {key: value for key, value in self.manifest.items() if key != 'evidence'}
        validate_manifest(without)
        evidence = self.manifest['evidence']
        for value in (None, {}, evidence | dict(extra='a' * 64), {k: v for k, v in evidence.items() if k != 'changes_sha256'},
                      evidence | dict(identity_sha256='sha256:' + 'a' * 64), evidence | dict(findings_sha256='A' * 64)):
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_manifest(self.manifest | dict(evidence=value))

    def test_evidence_names_the_uploaded_bytes(self):
        for name, field in (('findings.json', 'findings_sha256'), ('identity.jsonl', 'identity_sha256'), ('changes.jsonl', 'changes_sha256')):
            self.assertEqual(self.manifest['evidence'][field], digest((self.path / name).read_bytes()))


if __name__ == '__main__':
    unittest.main()
