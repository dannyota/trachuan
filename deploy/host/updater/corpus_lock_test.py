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
             mock.patch('updater.deploy_corpus_lock.os.read', return_value=b't\n'):
            with self.assertRaises(ValueError):
                with AdvisoryLock(db):
                    self.assertIs(db.holder, process)
                    raise ValueError('synthetic interruption')
        args = start.call_args.args[0]
        self.assertEqual(args[args.index('-U')+1], 'activator')
        process.stdin.close.assert_called_once()
        process.wait.assert_called_once_with(timeout=5)
        self.assertIsNone(db.holder)

    def test_busy_or_lost_lock_closes_session_without_mutation(self):
        for response, error in ((b'f\n', Busy), (b'', CommandError)):
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
