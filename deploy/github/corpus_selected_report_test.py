from repositories_test_support import enable_dual_policy as setUpModule

from contextlib import redirect_stdout
import copy
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from corpus_manifest import create, main, validate_inputs
from corpus_upload import upload
from corpus_test_support import digest, save_fixture, save_older_shape, write_evidence, write_fixture

REL = '20990102_1'


class SelectedReportTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.row, self.report = write_fixture(self.path)

    def upload(self):
        return validate_inputs('vn', REL, self.path, self.path / 'report.json')

    def sign(self):
        return create('dannyota/trachuan', 'a' * 40, 2, 'vn', REL, self.path, self.path / 'report.json')

    def save_run(self, **fields):
        row = copy.deepcopy(self.row)
        row['run'].update(fields)
        (self.path / 'release.json').write_text(json.dumps(row))

    def assert_both_refuse(self, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.upload()
        with self.assertRaisesRegex(ValueError, message):
            self.sign()

    def test_matching_hashes_pass_upload_and_signing(self):
        self.upload()
        self.sign()

    def test_hashes_must_match_report_and_its_previous(self):
        for fields in (dict(eval_report_sha256='0' * 64), dict(previous_report_sha256='0' * 64),
                       dict(eval_report_sha256=self.row['run']['eval_report_sha256'].upper()),
                       dict(previous_report_sha256=None), dict(eval_report_sha256=1)):
            with self.subTest(fields=fields):
                self.save_run(**fields)
                self.assert_both_refuse()

    def test_none_cannot_publish(self):
        for field, message in (('eval_report_sha256', 'differs from the selected report'),
                               ('previous_report_sha256', 'differs from the frozen previous report')):
            with self.subTest(field=field):
                self.save_run(**{field: 'none'})
                self.assert_both_refuse(message)

    def test_changed_report_bytes_are_refused(self):
        path = self.path / 'report.json'
        path.write_bytes(path.read_bytes() + b' ')
        self.assert_both_refuse()

    def test_one_field_alone_is_refused_even_at_the_cutoff(self):
        for field in ('eval_report_sha256', 'previous_report_sha256'):
            row = copy.deepcopy(self.row)
            row['run'].pop(field)
            (self.path / 'release.json').write_text(json.dumps(row))
            with self.subTest(field=field), patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL}):
                self.assert_both_refuse()

    def test_older_shape_above_the_cutoff_is_refused(self):
        save_older_shape(self.path, self.row)
        self.assert_both_refuse()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL})
    def test_older_shape_at_the_cutoff_passes_both(self):
        save_older_shape(self.path, self.row)
        self.upload()
        self.sign()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'})
    def test_older_shape_below_the_cutoff_passes_signing_only(self):
        save_older_shape(self.path, self.row)
        with self.assertRaises(ValueError):
            self.upload()
        self.sign()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': REL})
    def test_older_shape_skips_only_the_two_hash_checks(self):
        self.row['run']['unaccepted_gaps'] = 1
        save_older_shape(self.path, self.row)
        self.assert_both_refuse()
        self.row['run']['unaccepted_gaps'] = 0
        self.report['dump_sha256'] = digest(b'another dump')
        save_fixture(self.path, self.row, self.report)
        save_older_shape(self.path, self.row)
        self.assert_both_refuse()

    def run_cli(self, command, country='vn', list_files=False, check=True):
        environment = dict(GITHUB_REF='refs/heads/main', GITHUB_SHA='a' * 40, GITHUB_RUN_NUMBER='2', GITHUB_REPOSITORY='dannyota/trachuan')
        argv = ['corpus_manifest.py', command, country, REL, str(self.path), str(self.path / 'report.json')]
        argv += ['--list-files'] if list_files else []
        with patch.dict(os.environ, environment), patch('sys.argv', argv):
            if not check:
                main()
                return
            with patch('corpus_previous.check_remote', return_value=None):
                main()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'})
    def test_cli_applies_the_signing_rule_to_create_and_the_upload_rule_to_validate(self):
        save_older_shape(self.path, self.row)
        self.run_cli('create')
        self.assertTrue((self.path / 'manifest.json').exists())
        with self.assertRaises(ValueError):
            self.run_cli('validate')

    def test_cli_lists_and_signs_the_evidence_of_a_decision_release(self):
        output = io.StringIO()
        with redirect_stdout(output):
            self.run_cli('validate', list_files=True)
        self.assertEqual(output.getvalue().split(), ['corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json',
                                                     'findings.json', 'identity.jsonl', 'changes.jsonl'])
        with patch('corpus_previous.check_remote', return_value=None) as remote, redirect_stdout(io.StringIO()):
            self.run_cli('create', check=False)
        self.assertEqual(remote.call_args.kwargs['sidecars']['evidence'],
                         json.loads((self.path / 'manifest.json').read_bytes())['evidence'])

    def test_upload_refuses_a_gate_failure_before_any_remote_call(self):
        findings = json.loads((self.path / 'findings.json').read_bytes())
        findings['detectors']['unmatched_operative_claim']['complete'] = False
        write_evidence(self.path, self.row, self.report, findings=findings)
        with patch.dict(os.environ, OCI_S3_ENDPOINT='https://synthetic.invalid'), \
                patch('corpus_upload.require_settled') as settled, \
                patch('corpus_upload.subprocess.run') as run, \
                self.assertRaisesRegex(ValueError, 'detector not complete'):
            upload('vn', REL, self.path, self.path / 'report.json', 'dannyota/trachuan')
        settled.assert_not_called()
        run.assert_not_called()

    def test_validate_refuses_a_country_the_status_endpoint_does_not_serve(self):
        self.row['country'] = self.report['country'] = 'us'
        self.report['previous_diff']['previous']['country'] = 'us'
        save_fixture(self.path, self.row, self.report)
        with self.assertRaisesRegex(ValueError, 'public status'):
            self.run_cli('validate', country='us')


if __name__ == '__main__':
    unittest.main()
