import copy
import json
from pathlib import Path
import tempfile
import unittest

from corpus_manifest import create, validate_manifest
from corpus_test_support import add_heading_build, digest, save_fixture, write_fixture


class HeadingEvidenceTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.row, self.report = write_fixture(self.path)

    def create(self):
        return json.loads(create('dannyota/trachuan', 'a' * 40, 1, 'vn', self.row['rel'], self.path,
                                 self.path / 'report.json'))

    def test_schema_eight_copies_verified_build_values(self):
        record = add_heading_build(self.path, self.row, self.report)
        obj = self.create()
        self.assertEqual(obj.get('heading_evidence'), dict(projection=record['heading_projection'],
                         scan_sha256=record['heading_scan_sha256'],
                         exclusions_sha256=record['heading_exclusions_sha256']))
        self.assertEqual(validate_manifest(obj), obj)

    def test_schema_seven_does_not_require_or_read_build_record(self):
        self.row['schema_version'] = 7
        save_fixture(self.path, self.row, self.report)
        self.assertNotIn('heading_evidence', self.create())
        (self.path / 'build.json').write_text('invalid JSON')
        self.assertNotIn('heading_evidence', self.create())

    def test_build_record_missing_or_tampered_is_refused(self):
        for fault in ('missing', 'tampered', 'duplicate line', 'wrong hash', 'missing line'):
            with self.subTest(fault=fault):
                add_heading_build(self.path, self.row, self.report)
                path = self.path / 'build.json'
                manifest = self.path / 'MANIFEST.sha256'
                if fault == 'missing':
                    path.unlink()
                elif fault == 'tampered':
                    path.write_bytes(path.read_bytes().replace(b'a', b'c'))
                else:
                    lines = manifest.read_text().splitlines(keepends=True)
                    if fault == 'duplicate line':
                        lines.append(lines[-1])
                    elif fault == 'wrong hash':
                        lines[-1] = '0' * 64 + '  build.json\n'
                    else:
                        lines.pop()
                    manifest.write_text(''.join(lines))
                    self.report['build_artifact_sha256'] = self.row['run']['build_artifact_sha256'] = digest(manifest.read_bytes())
                    save_fixture(self.path, self.row, self.report)
                with self.assertRaises((ValueError, OSError)):
                    self.create()

    def test_verified_build_record_requires_well_typed_evidence(self):
        for field, value in [('heading_projection', None), ('heading_projection', True),
                             ('heading_projection', 1.0), ('heading_scan_sha256', 'BAD'),
                             ('heading_exclusions_sha256', 'B' * 64)]:
            with self.subTest(field=field, value=value):
                record = add_heading_build(self.path, self.row, self.report)
                if value is None:
                    record.pop(field)
                else:
                    record[field] = value
                data = json.dumps(record).encode()
                (self.path / 'build.json').write_bytes(data)
                path = self.path / 'MANIFEST.sha256'
                lines = path.read_text().splitlines(keepends=True)
                lines[-1] = digest(data) + '  build.json\n'
                path.write_text(''.join(lines))
                self.report['build_artifact_sha256'] = self.row['run']['build_artifact_sha256'] = digest(path.read_bytes())
                save_fixture(self.path, self.row, self.report)
                with self.assertRaises(ValueError):
                    self.create()

    def test_manifest_evidence_schema(self):
        obj = self.create()
        obj['schema_version'] = 8
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        obj['heading_evidence'] = evidence
        self.assertEqual(validate_manifest(obj), obj)
        cases = [None, [], {}, evidence | {'extra': 1}]
        for field in evidence:
            cases.append({key: value for key, value in evidence.items() if key != field})
        for field, value in [('projection', True), ('projection', '1'), ('projection', 1.0),
                             ('scan_sha256', 'A' * 64), ('scan_sha256', 'a' * 63),
                             ('exclusions_sha256', 'BAD'), ('exclusions_sha256', None)]:
            cases.append(evidence | {field: value})
        for value in cases:
            with self.subTest(evidence=value):
                candidate = copy.deepcopy(obj)
                candidate['heading_evidence'] = value
                with self.assertRaises(ValueError):
                    validate_manifest(candidate)

    def test_evidence_is_required_on_eight_and_forbidden_on_seven(self):
        obj = self.create()
        obj['schema_version'] = 8
        with self.assertRaises(ValueError):
            validate_manifest(obj)
        obj['schema_version'] = 7
        obj['heading_evidence'] = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        with self.assertRaises(ValueError):
            validate_manifest(obj)

    def test_review_is_copied_independently_of_evidence(self):
        add_heading_build(self.path, self.row, self.report)
        review = dict(release=self.row['rel'], projection=1, scan_sha256='c' * 64,
                      exclusions_sha256='d' * 64, reviewer='coordinator', date='2026-11-16')
        reviews = self.path / 'reviews.yaml'
        reviews.write_text(json.dumps([review]))
        obj = json.loads(create('dannyota/trachuan', 'a' * 40, 1, 'vn', self.row['rel'], self.path,
                               self.path / 'report.json', heading_reviews_path=reviews))
        self.assertEqual(obj['heading_review'], {key: value for key, value in review.items() if key != 'release'})
        self.assertEqual(obj.get('heading_evidence'), dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64))
