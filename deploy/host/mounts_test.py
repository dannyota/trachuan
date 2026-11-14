from pathlib import Path
import runpy
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class MountExtractionTest(unittest.TestCase):
    def setUp(self):
        runner = HOST / 'check-mounts.py'
        self.assertTrue(runner.exists(), 'mount runtime check is missing')
        self.check = runpy.run_path(str(runner))

    def test_every_quadlet_mount_keeps_container_user(self):
        with tempfile.TemporaryDirectory() as work:
            units = Path(work)
            (units / 'example.container').write_text(
                '[Container]\nUser=999:999\nTmpfs=/tmp:rw,mode=1777\n'
                'Mount=type=tmpfs,destination=/run/pgbackrest,tmpfs-mode=0700,U=true\n'
                'Tmpfs=/cache:rw,noexec\n[Service]\nUser=unrelated\n')
            self.assertEqual(self.check['quadlet_mounts'](units), [
                ('example.container', '999:999', '--tmpfs', '/tmp:rw,mode=1777'),
                ('example.container', '999:999', '--mount',
                 'type=tmpfs,destination=/run/pgbackrest,tmpfs-mode=0700,U=true'),
                ('example.container', '999:999', '--tmpfs', '/cache:rw,noexec'),
            ])

    def test_quadlet_secret_mounts_are_probed_with_their_options(self):
        with tempfile.TemporaryDirectory() as work:
            units = Path(work)
            (units / 'example.container').write_text(
                '[Container]\nUser=999:999\n'
                'Secret=activator_db_password,type=mount,uid=999,gid=999,mode=0400\n'
                'Secret=smtp_password,type=env,target=SERVER_SMTP_PASSWORD\n')
            mounts = self.check['quadlet_mounts'](units)
            self.assertEqual(mounts, [('example.container', '999:999', '--secret',
                                       'activator_db_password,type=mount,uid=999,gid=999,mode=0400')])
            script = self.check['probe_script'](mounts)
            self.assertIn('secret create activator_db_password -', script)
            self.assertIn('--secret activator_db_password,type=mount,uid=999,gid=999,mode=0400 ', script)
            self.assertIn('999:999 400', script)

    def test_script_mounts_accept_split_and_equals_arguments(self):
        with tempfile.TemporaryDirectory() as work:
            script = Path(work) / 'restore-check.sh'
            script.write_text(
                'podman rm --force example\n'
                'podman run --user 999:999 --tmpfs=/tmp:rw,mode=1777 \\\n'
                ' --mount "type=tmpfs,destination=/run/restore-check,tmpfs-mode=0700,U=true" \\\n'
                ' --tmpfs /cache:rw --mount=type=tmpfs,dst=/extra image true\n')
            self.assertEqual(self.check['script_mounts'](script), [
                ('restore-check.sh', '999:999', '--tmpfs', '/tmp:rw,mode=1777'),
                ('restore-check.sh', '999:999', '--mount',
                 'type=tmpfs,destination=/run/restore-check,tmpfs-mode=0700,U=true'),
                ('restore-check.sh', '999:999', '--tmpfs', '/cache:rw'),
                ('restore-check.sh', '999:999', '--mount', 'type=tmpfs,dst=/extra'),
            ])


    def test_each_source_must_yield_its_mounts(self):
        with self.assertRaises(ValueError):
            self.check['required']([], 'restore-check.sh', {'/run/restore-check'})
        mounts = [('restore-check.sh', '999:999', '--mount', 'type=tmpfs,destination=/run/restore-check,tmpfs-mode=0700,U=true')]
        self.assertEqual(self.check['required'](mounts, 'restore-check.sh', {'/run/restore-check'}), mounts)
        with self.assertRaises(ValueError):
            self.check['required'](mounts, 'restore-check.sh', {'/run/restore-check', '/tmp'})

if __name__ == '__main__':
    unittest.main()
