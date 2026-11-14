from contextlib import redirect_stderr
import io
import json
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater import main


class MainTest(fixture.ActivationFixture, unittest.TestCase):
    def test_invalid_policy_records_failure_without_resetting_state(self):
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.policy.parent.mkdir(parents=True, exist_ok=True)
        self.paths.policy.write_bytes(b'{invalid')
        before = self.state.path.read_bytes()
        with mock.patch.object(main, 'Paths', return_value=self.paths), \
             mock.patch.object(main.os, 'geteuid', return_value=0), \
             mock.patch.object(main.os, 'umask'), \
             mock.patch.object(main, 'trusted'), \
             mock.patch.object(main.sys, 'argv', ['trachuan-deploy', 'run']), \
             redirect_stderr(io.StringIO()):
            self.assertEqual(main.main(), 1)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'failed')


if __name__ == '__main__':
    unittest.main()
