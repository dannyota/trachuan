from unittest import mock
import unittest

from updater.deploy_corpus_lock import AdvisoryLock
from updater.deploy_corpus_database import Database
from updater.deploy_host import Host
from updater.deploy_store import Paths
from updater.deploy_io import Busy, CommandError


class AdvisoryLockTest(unittest.TestCase):
    def test_lock_uses_activator_session_and_releases_on_error(self):
        db = Database(Host(Paths(), user_prefix=[]), {})
        process = mock.Mock()
        process.poll.return_value = None
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.return_value = [(None,None)]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process) as start, \
             mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
             mock.patch('updater.deploy_corpus_lock.os.read', return_value=b't|4242\n'):
            with self.assertRaises(ValueError):
                with AdvisoryLock(db):
                    self.assertIs(db.holder, process)
                    self.assertEqual(db.backend, 4242)
                    raise ValueError('synthetic interruption')
        args = start.call_args.args[0]
        self.assertEqual(args[args.index('-U')+1], 'activator')
        self.assertIn('PGOPTIONS=-c search_path=pg_catalog -c idle_session_timeout=0', args)
        statement = process.stdin.write.call_args.args[0]
        self.assertIn(b'pg_catalog.pg_try_advisory_lock(pg_catalog.hashtextextended(', statement)
        self.assertIn(b'pg_catalog.pg_backend_pid()', statement)
        process.stdin.close.assert_called_once()
        process.wait.assert_called_once_with(timeout=5)
        self.assertIsNone(db.holder)

    def enter(self, reads):
        db = Database(Host(Paths(), user_prefix=[]), {})
        process = mock.Mock()
        process.poll.return_value = None
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.return_value = [(None, None)]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process), \
             mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
             mock.patch('updater.deploy_corpus_lock.os.read', side_effect=reads):
            with AdvisoryLock(db):
                return db.backend

    def test_backend_pid_is_read_to_the_end_of_its_line(self):
        self.assertEqual(self.enter([b't|42', b'42', b'\n']), 4242)

    def test_incomplete_or_extra_output_is_refused(self):
        for reads in ([b't|42', b''], [b't|4242\nextra\n'], [b't|4242\n\n']):
            with self.subTest(reads=reads), self.assertRaisesRegex(CommandError, 'advisory lock failed'):
                self.enter(reads)

    def test_silence_before_the_line_ends_times_out(self):
        db = Database(Host(Paths(), user_prefix=[]), {})
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.side_effect = [[(None, None)], []]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=mock.Mock()), \
             mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
             mock.patch('updater.deploy_corpus_lock.os.read', return_value=b't|42'):
            with self.assertRaisesRegex(CommandError, 'timed out'):
                with AdvisoryLock(db):
                    self.fail('incomplete answer accepted')

    def test_busy_or_lost_lock_closes_session_without_mutation(self):
        for response, error in ((b'f|4242\n', Busy), (b'', CommandError), (b't\n', CommandError), (b't|x\n', CommandError)):
            db = Database(Host(Paths(), user_prefix=[]), {})
            process = mock.Mock()
            selector = mock.MagicMock()
            selector.__enter__.return_value.select.return_value = [(None,None)]
            with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process), \
                 mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
                 mock.patch('updater.deploy_corpus_lock.os.read', return_value=response):
                with self.assertRaises(error):
                    with AdvisoryLock(db):
                        self.fail('unheld lock entered')
            process.stdin.close.assert_called_once()
            self.assertIsNone(db.holder)
