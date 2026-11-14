import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent
TOKEN = 'a'*32


class BackupJobTest(unittest.TestCase):
    def check_cancel(self, matching, unreadable=False):
        script = HOST/'backup-job.sh'
        self.assertTrue(script.exists(), 'container job supervisor is missing')
        with tempfile.TemporaryDirectory() as work:
            work = Path(work)
            job = work/'jobs'/TOKEN
            job.mkdir(parents=True)
            (job/'group').write_text('42\n')
            (job/'state').write_text('running\n')
            process = work/'proc'/'42'
            process.mkdir(parents=True)
            (process/'stat').write_text('42 (timeout) S 1 42 42 0\n')
            (process/'environ').write_bytes(('TRACHUAN_BACKUP_JOB='+ (TOKEN if matching else 'b'*32)+'\0').encode())
            if unreadable:
                (process/'environ').unlink()
            postgres = work/'proc'/'1'
            postgres.mkdir()
            (postgres/'stat').write_text('1 (postgres) S 0 1 1 0\n')
            (postgres/'environ').write_bytes(b'PGPASSFILE=/run/pgbackrest/pgpass\0')
            environment = work/'bash-env'
            environment.write_text('''flock() { :; }
kill() {
 printf '%s\\n' "$*" >>"$CALLS"
 [[ $* == '-TERM -- -42' || $* == '-KILL -- -42' ]] || return 90
 rm -rf "$BACKUP_PROC_ROOT/42"
}
''')
            env = dict(os.environ, BACKUP_JOB_ROOT=str(work/'jobs'), BACKUP_PROC_ROOT=str(work/'proc'),
                       BASH_ENV=str(environment), CALLS=str(work/'calls'))
            result = subprocess.run(['bash', str(script), 'cancel', TOKEN], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 125 if unreadable else 0, result.stderr)
            self.assertEqual(result.stdout, '' if unreadable else 'stopped\n')
            calls = (work/'calls').read_text().splitlines() if (work/'calls').exists() else []
            self.assertEqual(calls, ['-TERM -- -42'] if matching and not unreadable else [])
            self.assertTrue(postgres.exists())
            delayed = subprocess.run(['bash', str(script), 'run', TOKEN, '60', 'pgbackrest', 'verify'],
                                     env=env, capture_output=True, text=True)
            self.assertNotEqual(delayed.returncode, 0)

    def test_prepare_never_overwrites_cancellation_that_won_the_start_lock(self):
        with tempfile.TemporaryDirectory() as work:
            work = Path(work)
            environment = work/'bash-env'
            environment.write_text('flock() { printf "cancelled\\n" >"$dir/state"; }\n')
            env = dict(os.environ, BACKUP_JOB_ROOT=str(work/'jobs'), BASH_ENV=str(environment))
            result = subprocess.run(['bash', str(HOST/'backup-job.sh'), 'prepare', TOKEN],
                                    env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual((work/'jobs'/TOKEN/'state').read_text(), 'cancelled\n')

    def test_cancel_targets_only_the_matching_job_group(self):
        self.check_cancel(matching=True)

    def test_unreadable_job_identity_never_confirms_exit(self):
        self.check_cancel(matching=True, unreadable=True)

    def test_reused_pid_without_job_marker_is_never_signalled(self):
        self.check_cancel(matching=False)


if __name__ == '__main__':
    unittest.main()
