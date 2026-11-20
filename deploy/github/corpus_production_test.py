import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import corpus_production
from corpus_test_support import FakeCurl

PREVIOUS, REL = '20990101_1', '20990102_1'
URL = 'https://trachuan.vn/api/v1/public/status'


def manifest(rel=PREVIOUS, sequence=9, report='b' * 64):
    return dict(country='vn', rel=rel, sequence=sequence, report=dict(sha256=report, quality_pass=True))


def report(rel=PREVIOUS, digest='b' * 64, country='vn'):
    return dict(country=country, rel=REL,
                previous_diff=dict(previous=dict(country=country, rel=rel, report_sha256=digest)))


class ProductionTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        self.log = self.path / 'calls.jsonl'
        self.curl = FakeCurl(self.path, self.log)
        search_path = patch.dict(os.environ, PATH=f"{self.path}{os.pathsep}{os.environ['PATH']}")
        search_path.start()
        self.addCleanup(search_path.stop)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def settled(self, candidate=None, current=None, **answer):
        self.curl.answer(**({'release': PREVIOUS} | answer))
        with patch('corpus_production.production', return_value=current or manifest()) as production:
            result = corpus_production.require_settled(candidate or report(), self.path, 'dannyota/trachuan')
        return result, production

    def test_settled_production_passes_with_the_exact_status_request(self):
        result, production = self.settled()
        self.assertEqual(result['rel'], PREVIOUS)
        self.assertEqual(production.call_args.args[:2], ('dannyota/trachuan', 'vn'))
        self.assertEqual(self.calls()[0]['args'],
                         ['-q', '--silent', '--show-error', '--noproxy', '*', '--proto', '=https',
                          '--max-redirs', '0', '--max-time', '30', '--max-filesize', str(1024**2),
                          '--header', 'Cache-Control: no-cache', '--output', '-',
                          '--write-out', '%{http_code}', URL])

    def test_status_request_runs_with_only_the_search_path(self):
        hostile = dict(TRACHUAN_URL='https://example.invalid', STATUS_URL='https://example.invalid',
                       CURL_CA_BUNDLE='/nonexistent/ca.pem', SSL_CERT_FILE='/nonexistent/ca.pem',
                       HTTPS_PROXY='http://127.0.0.1:9', https_proxy='http://127.0.0.1:9',
                       ALL_PROXY='http://127.0.0.1:9', all_proxy='http://127.0.0.1:9',
                       NO_PROXY='*', no_proxy='*', CURL_HOME='/nonexistent')
        with patch.dict(os.environ, hostile), \
                patch('corpus_production.subprocess.Popen', wraps=subprocess.Popen) as popen:
            self.settled()
        self.assertEqual(popen.call_args.kwargs['env'], {'PATH': os.environ['PATH']})
        self.assertEqual(self.calls()[0]['args'][-1], URL)
        self.assertFalse(set(hostile) & set(self.calls()[0]['environment']))

    def test_response_at_the_limit_passes_without_a_content_length(self):
        body = json.dumps(dict(release=PREVIOUS)).encode()
        with patch('corpus_production.STATUS_LIMIT', len(body) + 8):
            self.settled(body=body + b' ' * 8)

    def test_response_over_the_limit_is_refused_without_a_content_length(self):
        body = json.dumps(dict(release=PREVIOUS)).encode()
        with patch('corpus_production.STATUS_LIMIT', len(body) + 7), \
                self.assertRaisesRegex(ValueError, 'size limit'):
            self.settled(body=body + b' ' * 8)

    def test_refuses_a_report_not_compared_with_production_before_asking_the_site(self):
        for candidate, current in ((report(rel='20990101_2'), None), (report(digest='0' * 64), None),
                                   (None, manifest(rel='20990101_2')), (None, manifest(report='0' * 64))):
            with self.subTest(candidate=candidate, current=current), self.assertRaises(ValueError):
                self.settled(candidate, current)
        self.assertEqual(self.calls(), [])

    def test_refuses_when_no_production_manifest_exists(self):
        with patch('corpus_production.production', return_value=None), self.assertRaises(ValueError):
            corpus_production.require_settled(report(), self.path, 'dannyota/trachuan')
        self.assertEqual(self.calls(), [])

    def test_refuses_any_other_status_answer(self):
        answers = [dict(code='503'), dict(code='301'), dict(code='304'), dict(code='000'), dict(code=''),
                   dict(body=json.dumps(dict(release='20990101_2')).encode()),
                   dict(body=b'not json'), dict(body=b'[]'), dict(body=b'{}'),
                   dict(body=b'{"release":null}'), dict(body=b'{"release":1}'), dict(body=b'{"release":""}'),
                   dict(body=b'{"release":"20990101_1","release":"20990101_1"}'), dict(body=b'\xff')]
        for answer in answers:
            with self.subTest(answer=answer), self.assertRaises(ValueError):
                self.settled(**answer)

    def test_transport_failure_refuses(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.settled(exit_code=7)

    def test_other_country_is_refused_without_any_call(self):
        with patch('corpus_production.production') as production, self.assertRaises(ValueError):
            corpus_production.require_settled(report(country='us'), self.path, 'dannyota/trachuan')
        production.assert_not_called()
        self.assertEqual(self.calls(), [])

    def signable(self, candidate, current, existing=None, existing_error=None):
        run = dict(eval_report_sha256='e' * 64, previous_report_sha256='b' * 64)
        with patch('corpus_production.production', return_value=current), \
                patch('corpus_production.existing_release', return_value=existing, side_effect=existing_error):
            return corpus_production.require_signable(manifest(rel=REL, sequence=11), candidate, run, self.path, 'dannyota/trachuan')

    def test_first_signing_requires_the_comparison(self):
        self.assertEqual(self.signable(report(), manifest())['rel'], PREVIOUS)
        for candidate, current in ((report(rel='20990101_2'), manifest()), (report(digest='0' * 64), manifest()),
                                   (report(), manifest(rel='20990101_2')), (report(), manifest(report='0' * 64))):
            with self.subTest(candidate=candidate, current=current), self.assertRaises(ValueError):
                self.signable(candidate, current)

    def test_retry_of_a_failed_activation_is_exempt(self):
        current = manifest(rel=REL, sequence=10, report='c' * 64)
        self.assertIs(self.signable(report(), current, existing=manifest(rel=REL, sequence=10)), current)

    def test_rollback_by_signing_an_older_release_is_exempt(self):
        current = manifest(rel='20990103_1', sequence=12, report='c' * 64)
        self.assertIs(self.signable(report(), current, existing=manifest(rel=REL, sequence=10)), current)

    def test_push_without_production_move_is_not_exempt(self):
        existing = manifest(rel=REL, sequence=10)
        self.assertEqual(self.signable(report(), manifest(sequence=9), existing=existing)['rel'], PREVIOUS)
        moved = manifest(rel='20990101_2', sequence=9, report='c' * 64)
        with self.assertRaises(ValueError):
            self.signable(report(), moved, existing=existing)

    def test_unverified_existing_release_refuses(self):
        with self.assertRaises(ValueError):
            self.signable(report(), manifest(), existing_error=ValueError('signature'))

    def test_signing_without_production_refuses(self):
        with self.assertRaises(ValueError):
            self.signable(report(), None, existing=manifest(rel=REL, sequence=1))


if __name__ == '__main__':
    unittest.main()
