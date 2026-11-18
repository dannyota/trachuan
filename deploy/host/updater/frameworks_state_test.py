import copy
import json
from pathlib import Path
import tempfile
import unittest

from updater import deploy_state as s


def section(**values):
    return dict(dict(accepted={}, retained=[], failed=[]), **values)


class FrameworksStateTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name) / 'state.json'
        self.state = s.State.bootstrap(self.path, 'a' * 64, 14, 7, 'b' * 64)

    def test_key_appears_only_with_first_frameworks_acceptance(self):
        self.assertEqual(self.state.records('frameworks'), {})
        self.assertEqual(self.state.failed('frameworks'), [])
        self.state.save()
        self.assertNotIn('frameworks', json.loads(self.path.read_bytes()))
        self.assertEqual(self.state.accept('frameworks', 'c' * 64, 1), 'new')
        saved = json.loads(self.path.read_bytes())
        self.assertEqual(saved['frameworks'], section(accepted={'1': {'hash': 'c' * 64, 'status': 'pending'}}))
        self.assertEqual(saved['accepted']['corpus'], {})

    def test_frameworks_key_is_validated_as_closed_shape(self):
        good = section(accepted={'2': {'hash': 'c' * 64, 'status': 'retained'}}, retained=['c' * 64], failed=['d' * 64])
        self.state.data['frameworks'] = good
        self.state.save()
        self.assertEqual(s.State(self.path).data['frameworks'], good)
        for bad in (None, [], section(extra=[]), {'accepted': {}, 'failed': []},
                    section(accepted={'0': {'hash': 'c' * 64, 'status': 'pending'}}),
                    section(accepted={'1': {'hash': 'c' * 64, 'status': 'consumed'}}),
                    section(accepted={'1': {'hash': 'bad', 'status': 'pending'}}),
                    section(accepted={'1': {'hash': 'c' * 64, 'status': 'pending', 'extra': 1}}),
                    section(retained=['bad']), section(failed='d' * 64)):
            data = dict(copy.deepcopy(self.state.data), frameworks=bad)
            self.path.write_text(json.dumps(data))
            with self.subTest(bad=bad), self.assertRaisesRegex(ValueError, 'bootstrap'):
                s.State(self.path)

    def test_sequences_and_failures_are_independent_of_law_corpus(self):
        self.state.accept('corpus', 'c' * 64, 5)
        self.assertEqual(self.state.accept('frameworks', 'e' * 64, 2), 'new')
        self.assertEqual(self.state.accept('frameworks', 'd' * 64, 5), 'new')
        with self.assertRaisesRegex(ValueError, 'older'):
            self.state.accept('frameworks', 'f' * 64, 1)
        with self.assertRaisesRegex(ValueError, 'reuse'):
            self.state.accept('frameworks', 'f' * 64, 5)
        self.state.fail('d' * 64, 'frameworks')
        reloaded = s.State(self.path)
        self.assertEqual(reloaded.failed('frameworks'), ['d' * 64])
        self.assertEqual(reloaded.data['failed'], [])
        self.assertEqual(reloaded.decision('frameworks', 'd' * 64, 5), 'noop')
        self.assertEqual(reloaded.decision('corpus', 'c' * 64, 5), 'pending')
        self.assertEqual(reloaded.records('corpus')['5']['status'], 'pending')
        self.state.fail('c' * 64, 'corpus')
        self.assertEqual(s.State(self.path).data['failed'], ['c' * 64])
        self.assertEqual(s.State(self.path).failed('frameworks'), ['d' * 64])
        self.assertEqual(s.State(self.path).decision('frameworks', 'c' * 64, 6), 'new')

    def test_settle_marks_active_retains_previous_and_fails_lower_pending(self):
        for sequence, digest in ((1, 'c'), (2, 'd'), (3, 'e')):
            self.state.accept('frameworks', digest * 64, sequence)
        self.state.settle('frameworks', 'c' * 64, 1, True)
        self.state.settle('frameworks', 'e' * 64, 3, True)
        reloaded = s.State(self.path)
        statuses = {n: r['status'] for n, r in reloaded.records('frameworks').items()}
        self.assertEqual(statuses, {'1': 'retained', '2': 'failed', '3': 'active'})
        self.assertEqual(reloaded.data['frameworks']['retained'], ['c' * 64])
        self.assertEqual(reloaded.failed('frameworks'), ['d' * 64])
        self.assertEqual(reloaded.decision('frameworks', 'c' * 64, 1), 'noop')
        self.assertEqual(reloaded.data['accepted']['corpus'], {})
        self.assertEqual(reloaded.data['failed'], [])
        self.state.settle('frameworks', 'c' * 64, 1, True)
        self.assertEqual(s.State(self.path).data['frameworks']['retained'], ['e' * 64])

    def test_law_settle_keeps_existing_corpus_bookkeeping(self):
        for sequence, digest in ((1, 'c'), (2, 'd'), (3, 'e')):
            self.state.accept('corpus', digest * 64, sequence)
        self.state.settle('corpus', 'c' * 64, 1, True)
        self.state.settle('corpus', 'e' * 64, 3, True)
        reloaded = s.State(self.path)
        self.assertEqual({n: r['status'] for n, r in reloaded.records('corpus').items()},
                         {'1': 'retained', '2': 'failed', '3': 'active'})
        self.assertEqual(reloaded.data['retained'], [])
        self.assertEqual(reloaded.data['failed'], ['d' * 64])
        self.assertNotIn('frameworks', reloaded.data)


if __name__ == '__main__':
    unittest.main()
