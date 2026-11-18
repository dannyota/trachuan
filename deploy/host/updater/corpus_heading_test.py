import copy
from pathlib import Path
import unittest

from updater.deploy_corpus_heading import headings_approved
from updater.corpus_verify_test import manifest as sample_manifest
from updater.deploy_verify import policy
from updater.deploy_corpus_verify import manifest
from updater.deploy_io import canonical


class HeadingTest(unittest.TestCase):
    def test_approval_requires_both_matching_records(self):
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        review = dict(evidence, reviewer='coordinator', date='2026-11-16')
        for field in (None, 'projection', 'scan_sha256', 'exclusions_sha256'):
            with self.subTest(field=field):
                obj = dict(heading_evidence=copy.deepcopy(evidence), heading_review=review)
                if field:
                    obj['heading_evidence'][field] = 2 if field == 'projection' else 'c' * 64
                self.assertEqual(headings_approved(obj), field is None)
        self.assertFalse(headings_approved({}))
        self.assertFalse(headings_approved(dict(heading_review=review)))
        self.assertFalse(headings_approved(dict(heading_evidence=evidence)))

    def test_signed_manifest_optional_evidence(self):
        rules = policy(Path(__file__).parents[1] / 'deploy-policy.json')
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        for fields in ({}, {'heading_evidence': evidence}, {'heading_review': dict(evidence, reviewer='coordinator', date='2026-11-16')}, {'heading_evidence': evidence, 'heading_review': dict(evidence, reviewer='coordinator', date='2026-11-16')}):
            obj = dict(sample_manifest(), schema_version=8 if 'heading_evidence' in fields else 6, **fields)
            with self.subTest(fields=fields):
                self.assertEqual(manifest(canonical(obj), rules), obj)
        for value in (None, {}, dict(evidence, extra=True), dict(evidence, projection=True), dict(evidence, scan_sha256='bad')):
            with self.subTest(value=value), self.assertRaises(ValueError):
                manifest(canonical(dict(sample_manifest(), schema_version=8, heading_evidence=value)), rules)

    def test_activation_records_capability_and_approval(self):
        from updater.deploy_corpus_sql import activate
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        obj = dict(sample_manifest(), schema_version=8, heading_evidence=evidence,
                   heading_review=dict(evidence, reviewer='coordinator', date='2026-11-16'))
        row = dict(country='vn', database_name=obj['database'], expected_active_db='corpus_vn_20990101_1',
                   mode='new', manifest=obj, operation_id='e'*32, rel=obj['rel'], manifest_hash='a'*64,
                   dump_sha256=obj['dump']['sha256'])
        self.assertIn('summary_projection=1,headings_approved=true', activate(row, 1))
        self.assertIn('summary_projection=0,headings_approved=false', activate(row, 0))
        obj.pop('heading_evidence')
        self.assertIn('summary_projection=1,headings_approved=false', activate(row, 1))

    def test_evidence_follows_schema_version(self):
        rules = policy(Path(__file__).parents[1] / 'deploy-policy.json')
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        for schema, supplied, valid in ((6, False, True), (6, True, False), (8, False, False), (8, True, True)):
            obj = dict(sample_manifest(), schema_version=schema)
            if supplied:
                obj['heading_evidence'] = evidence
            with self.subTest(schema=schema, supplied=supplied):
                if valid:
                    self.assertEqual(manifest(canonical(obj), rules), obj)
                else:
                    with self.assertRaises(ValueError):
                        manifest(canonical(obj), rules)

    def test_activation_probes_target_capability_before_recording_approval(self):
        import json
        from updater.deploy_corpus_database import Database
        evidence = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)
        obj = dict(sample_manifest(), schema_version=8, heading_evidence=evidence,
                   heading_review=dict(evidence, reviewer='coordinator', date='2026-11-16'))
        row = dict(country='vn', database_name=obj['database'], expected_active_db='corpus_vn_20990101_1',
                   mode='new', manifest=obj, operation_id='e'*32, rel=obj['rel'], manifest_hash='a'*64,
                   dump_sha256=obj['dump']['sha256'])
        for present, supported in ((False, False), (True, False), (True, True)):
            calls = []
            def query(text, database='auth', options=None):
                calls.append((text, database))
                if database == 'auth':
                    return json.dumps(dict(row, step='activated'))
                self.assertEqual(database, obj['database'])
                self.assertEqual(options, '-c search_path=pg_catalog')
                if text == "SELECT pg_catalog.to_regclass('public.document_summary') IS NOT NULL;":
                    return 't' if present else 'f'
                if text == 'SELECT NOT EXISTS(SELECT FROM public.document_summary WHERE projection <> 1);':
                    return 't' if supported else 'f'
                self.fail('unexpected capability query')
            db = Database(None, None)
            db.query = query
            db.target_oid = lambda row: None
            with self.subTest(present=present, supported=supported):
                self.assertEqual(db.activate(row)['step'], 'activated')
                value = 'summary_projection=1,headings_approved=true' if present and supported else 'summary_projection=0,headings_approved=false'
                self.assertIn(value, calls[-1][0])
