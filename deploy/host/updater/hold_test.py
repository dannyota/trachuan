from pathlib import Path
import shlex
import subprocess
import unittest

from updater import activation_test as fixture
from updater.deploy_io import CommandError
from updater.deploy_state import State


class HoldTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.hold = self.paths.root / 'srv/trachuan/state/deploy-hold'

    def stopped(self):
        self.crash_step = 'migrate'
        with self.assertRaises(fixture.Crash):
            self.engine.activate(self.new)
        self.schema = 8
        self.assertEqual(self.engine.recover(), 'stopped')

    def test_stopped_journal_retains_hold_and_repeatedly_stops_caddy(self):
        self.stopped()
        self.assertTrue(self.hold.is_file())
        self.assertEqual(self.hold.stat().st_mode & 0o777, 0o644)
        self.hold.unlink()
        self.calls.clear()
        self.assertIsNone(self.engine.recover())
        self.assertTrue(self.hold.is_file())
        self.assertEqual(self.calls[0][0], ['systemctl', '--user', 'stop', 'trachuan-caddy.service'])
        self.assertEqual(State(self.state.path).data['journal']['status'], 'stopped')

    def test_caddy_startup_gate_rejects_hold_until_it_is_removed(self):
        unit = Path(__file__).parents[1] / 'quadlet/trachuan-caddy.container'
        guards = [shlex.split(line.split('=', 1)[1]) for line in unit.read_text().splitlines()
                  if line.startswith('ExecStartPre=/usr/bin/test ')]
        self.assertEqual(len(guards), 1)
        self.assertEqual(guards[0], ['/usr/bin/test', '!', '-e', '/srv/trachuan/state/deploy-hold'])
        args = guards[0][:-1] + [str(self.hold)]
        command = ['bash', '-c', 'test "$@"', '--', *args[1:]]
        self.assertEqual(subprocess.run(command).returncode, 0)
        self.hold.write_bytes(b'held\n')
        self.assertNotEqual(subprocess.run(command).returncode, 0)
        self.hold.unlink()
        self.assertEqual(subprocess.run(command).returncode, 0)

    def test_activation_clears_hold_only_between_readiness_and_caddy_start(self):
        self.hold.write_bytes(b'held\n')
        ready = []
        def runner(args, **kwargs):
            if args[-1] == 'http://127.0.0.1:8080/readyz':
                self.assertTrue(self.hold.exists())
                ready.append(True)
            if 'start' in args and 'trachuan-caddy.service' in args:
                self.assertTrue(ready)
                self.assertFalse(self.hold.exists())
            return self.runner(args, **kwargs)
        self.host.runner = runner
        self.engine.activate(self.new)
        self.assertFalse(self.hold.exists())

    def test_caddy_start_edge_and_later_schema_failures_restore_hold(self):
        for stage in ('caddy', 'edge', 'schema'):
            with self.subTest(stage=stage):
                self.setUp()
                self.hold.write_bytes(b'held\n')
                probes = []
                def runner(args, input=None, **kwargs):
                    if args[-1] == 'http://127.0.0.1:8080/readyz':
                        probes.append(True)
                    fail = (stage == 'caddy' and 'start' in args and 'trachuan-caddy.service' in args
                            or stage == 'edge' and args[-1] == 'https://trachuan.vn/'
                            or stage == 'schema' and probes and input and b'max(version_id)' in input)
                    if fail:
                        self.assertFalse(self.hold.exists())
                        raise CommandError('injected later failure')
                    return self.runner(args, input=input, **kwargs)
                self.host.runner = runner
                with self.assertRaises(CommandError):
                    self.engine.activate(self.new)
                self.assertTrue(self.hold.is_file())

    def test_recovery_holds_before_first_stop_and_successful_rollback_clears_it(self):
        self.crash_step = 'migrate'
        with self.assertRaises(fixture.Crash):
            self.engine.activate(self.new)
        self.hold.unlink(missing_ok=True)
        def runner(args, **kwargs):
            if 'stop' in args and 'trachuan-caddy.service' in args:
                self.assertTrue(self.hold.is_file())
            return self.runner(args, **kwargs)
        self.host.runner = runner
        self.assertEqual(self.engine.recover(), 'rolled_back')
        self.assertFalse(self.hold.exists())


if __name__ == '__main__':
    unittest.main()
