import json
from pathlib import Path
import tempfile
import unittest

from corpus_fixture_history import FixtureHistory, development_cases
from corpus_test_support import digest, fixture_repository, git


def version(*ids, tag='identifier'):
    return json.dumps([dict(tags=[tag], id=case_id, question='q-' + case_id,
                            expected_citations=[dict(doc_number='doc')]) for case_id in ids]).encode()


A, B, C = version('a'), version('a', 'b'), version('a', 'b', 'c')


class FixtureHistoryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)

    def history(self, versions, branch='main'):
        return FixtureHistory(fixture_repository(self.path / 'repo', versions, branch))

    def test_finds_each_version_by_hash(self):
        history = self.history([A, B, C])
        for data in (A, B, C):
            self.assertEqual(history.fixture(digest(data)), data)

    def test_refuses_a_hash_with_no_version_in_main(self):
        history = self.history([A, B])
        with self.assertRaisesRegex(ValueError, 'not in the history of the local main branch'):
            history.fixture(digest(C))

    def test_reads_main_not_the_checked_out_branch(self):
        history = self.history([A])
        root = self.path / 'repo'
        git(root, 'switch', '--quiet', '--create', 'feature')
        (root / 'apps/corpus/internal/evaluation/testdata/vn/development.json').write_bytes(B)
        git(root, 'commit', '--quiet', '--all', '-m', 'feature')
        with self.assertRaisesRegex(ValueError, 'not in the history of the local main branch'):
            history.fixture(digest(B))

    def test_refuses_a_shallow_repository(self):
        self.history([A, B])
        clone = self.path / 'clone'
        git(self.path, 'clone', '--quiet', '--depth', '1', (self.path / 'repo').as_uri(), str(clone))
        with self.assertRaisesRegex(ValueError, 'shallow'):
            FixtureHistory(clone).fixture(digest(B))

    def test_refuses_a_missing_main_ref(self):
        history = self.history([A], branch='trunk')
        with self.assertRaisesRegex(ValueError, 'main'):
            history.fixture(digest(A))

    def test_refuses_a_directory_without_history(self):
        with self.assertRaises(ValueError):
            FixtureHistory(self.path).fixture(digest(A))

    def test_candidate_must_be_at_or_after_the_previous_version(self):
        history = self.history([A, B, C])
        history.require_order(digest(C), digest(A))
        history.require_order(digest(B), digest(B))
        with self.assertRaisesRegex(ValueError, 'older'):
            history.require_order(digest(A), digest(B))

    def test_latest_occurrence_orders_a_reverted_version(self):
        history = self.history([A, B, A])
        history.require_order(digest(A), digest(B))
        with self.assertRaisesRegex(ValueError, 'older'):
            history.require_order(digest(B), digest(A))

    def test_stale_candidate_is_refused(self):
        history = self.history([A, B])
        with self.assertRaisesRegex(ValueError, 'older'):
            history.require_order(digest(A), digest(B))
        with self.assertRaisesRegex(ValueError, 'not in the history'):
            history.require_order(digest(C), digest(B))

    def test_deleted_fixture_is_skipped(self):
        history = self.history([A, None, B])
        self.assertEqual(history.fixture(digest(A)), A)
        history.require_order(digest(B), digest(A))

    def test_ignores_git_environment_of_the_caller(self):
        import os
        from unittest.mock import patch
        history = self.history([A])
        with patch.dict(os.environ, GIT_DIR=str(self.path / 'missing')):
            self.assertEqual(history.fixture(digest(A)), A)


class DevelopmentCasesTest(unittest.TestCase):
    def test_decodes_cases_and_refuses_unknown_shapes(self):
        self.assertEqual([case['id'] for case in development_cases(C)], ['a', 'b', 'c'])
        for data in (b'{}', b'[1]', b'[{"id":"a","question":"q","tags":["identifier"],"Question":"x"}]',
                     b'[{"id":"a","question":"q","tags":["identifier"]},{"id":"a","question":"q","tags":["identifier"]}]',
                     b'[{"id":"a","question":"q"}]', b'[{"id":"a","question":"q","tags":["other"]}]',
                     b'[{"id":"a","question":"q","tags":["identifier"],"expected_citations":[{"doc_number":"d","Article":"1"}]}]',
                     b'[{"id":"a","question":"q","tags":["identifier"],"expect_abstain":1}]',
                     b'[{"id":"a","id":"b","question":"q","tags":["identifier"]}]'):
            with self.subTest(data=data), self.assertRaises(ValueError):
                development_cases(data)


if __name__ == '__main__':
    unittest.main()
