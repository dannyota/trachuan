import configparser
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


HOST = Path(__file__).parents[1]


class ToolInstallTest(unittest.TestCase):
    def test_interrupted_updater_install_keeps_complete_previous_modules(self):
        helper = HOST / 'updater/install-updater.sh'
        self.assertTrue(helper.is_file())
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            source = work / 'source/updater'
            shutil.copytree(HOST / 'updater', source)
            target = work / 'libexec'
            subprocess.run(['bash', str(helper), str(source), str(target)], check=True)
            previous = (target / 'updater').resolve()
            self.assertTrue((target / 'updater').is_symlink())
            self.assertEqual((previous / 'fixtures/manifest.json').read_bytes(),
                             (source / 'fixtures/manifest.json').read_bytes())
            before = (previous / 'deploy_run.py').read_bytes()
            (source / 'deploy_run.py').write_bytes(before + b'\n')
            commands = work / 'commands'
            commands.mkdir()
            install = commands / 'install'
            install.write_text('#!/bin/bash\nfor arg in "$@"; do [[ $arg != *deploy_run.py ]] || exit 9; done\nexec ' + shutil.which('install') + ' "$@"\n')
            install.chmod(0o755)
            result = subprocess.run(['bash', str(helper), str(source), str(target)],
                env=dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH']), capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual((target / 'updater').resolve(), previous)
            self.assertEqual((previous / 'deploy_run.py').read_bytes(), before)
            self.assertEqual(list((target / 'updaters').glob('.staging.*')), [])

    def test_checksum_failure_never_installs_a_tool(self):
        self.assertTrue((HOST / 'updater/install-tools.sh').is_file())
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            commands = work / 'commands'
            commands.mkdir()
            for name, source in {
                'uname': '#!/bin/bash\nif [[ $1 == -s ]]; then echo Linux; else echo aarch64; fi\n',
                'curl': '#!/bin/bash\nwhile [[ $1 != -o ]]; do shift; done\nprintf untrusted >"$2"\n',
                'sha256sum': '#!/bin/bash\ncat >/dev/null\nexit 1\n',
            }.items():
                script = commands / name
                script.write_text(source)
                script.chmod(0o755)
            target = work / 'target'
            result = subprocess.run(['bash', str(HOST / 'updater/install-tools.sh'), str(target)],
                env=dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH']),
                capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((target / 'cosign').exists())
            self.assertFalse((target / 'oras').exists())

    def test_root_unit_and_timer_allow_owner_controlled_mode(self):
        service = configparser.ConfigParser(interpolation=None)
        service.read(HOST / 'systemd/trachuan-deploy.service')
        timer = configparser.ConfigParser(interpolation=None)
        timer.read(HOST / 'systemd/trachuan-deploy.timer')
        self.assertEqual(service['Service']['Type'], 'oneshot')
        self.assertEqual(service['Service']['User'], 'root')
        self.assertEqual(service['Unit']['ConditionPathExists'], '/etc/trachuan/deploy-mode')
        self.assertEqual(service['Service']['ExecStart'], '/usr/local/libexec/trachuan/trachuan-deploy run')
        self.assertEqual(timer['Timer']['OnBootSec'], '2min')
        self.assertEqual(timer['Timer']['OnUnitActiveSec'], '5min')


if __name__ == '__main__':
    unittest.main()
