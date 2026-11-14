import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


HOST = Path(__file__).parents[1]


def archive(data):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as tar:
        member = tarfile.TarInfo('oras')
        member.size = len(data)
        tar.addfile(member, io.BytesIO(data))
    return output.getvalue()


class ToolCacheTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)
        self.source = self.root / 'bundle/updater'
        shutil.copytree(HOST / 'updater', self.source)
        self.target = self.root / 'bin'
        self.target.mkdir()
        self.payloads = self.root / 'payloads'
        self.payloads.mkdir()
        self.cosign, self.oras = b'synthetic cosign', b'synthetic oras'
        self.tar = archive(self.oras)
        for name, data in (('cosign', self.cosign), ('oras.tar.gz', self.tar)):
            (self.payloads / name).write_bytes(data)
        for name, data in (('cosign', self.cosign), ('oras', self.oras)):
            (self.target / name).write_bytes(data)
            (self.target / name).chmod(0o755)
        self.script = self.source / 'install-tools.sh'
        source = self.script.read_text()
        for pin, data in (
            ('ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2', self.cosign),
            ('e61243d2c862a5aaf6451914edd8cc089ae91df425201fa9ee7659cbed36e7c0', self.oras),
            ('b84269e78dfc2d5329a4aab373da3200de95e6c138338cf6c66c10c0a3f248c1', self.tar)):
            source = source.replace(pin, hashlib.sha256(data).hexdigest())
        self.script.write_text(source)
        self.commands = self.root / 'commands'
        self.commands.mkdir()
        commands = {
            'uname': '#!/bin/bash\nif [[ $1 == -s ]]; then echo Linux; else echo aarch64; fi\n',
            'rpm': '#!/bin/bash\nexit 0\n',
            'dnf': '#!/bin/bash\nexit 89\n',
            'curl': r'''#!/usr/bin/env python3
import os
from pathlib import Path
import shutil
import sys
root = Path(os.environ['TOOL_TEST_ROOT'])
with (root / 'requests').open('a') as output:
    output.write(sys.argv[-1] + '\n')
if os.environ.get('TOOL_TEST_OFFLINE') == '1':
    sys.exit(88)
target = sys.argv[sys.argv.index('-o') + 1]
shutil.copyfile(root / 'payloads' / Path(target).name, target)
''',
        }
        for name, data in commands.items():
            path = self.commands / name
            path.write_text(data)
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.commands) + os.pathsep + os.environ['PATH'],
                        TOOL_TEST_ROOT=str(self.root))

    def install(self, offline=False):
        return subprocess.run(['bash', str(self.script), str(self.target)],
            env=dict(self.env, TOOL_TEST_OFFLINE='1' if offline else '0'), capture_output=True, text=True)

    def test_valid_cached_executables_are_reused_without_network_or_replacement(self):
        before = {p.name: p.stat().st_ino for p in self.target.iterdir()}
        result = self.install(offline=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / 'requests').exists())
        self.assertEqual({p.name: p.stat().st_ino for p in self.target.iterdir()}, before)

    def test_only_corrupt_tool_is_downloaded_then_both_work_offline(self):
        for damaged in ('cosign', 'oras'):
            with self.subTest(damaged=damaged):
                self.setUp()
                (self.target / damaged).write_bytes(b'corrupt')
                result = self.install()
                self.assertEqual(result.returncode, 0, result.stderr)
                requests = (self.root / 'requests').read_text().splitlines()
                self.assertEqual(len(requests), 1)
                self.assertIn('cosign-linux-arm64' if damaged == 'cosign' else 'oras_1.3.6_linux_arm64.tar.gz', requests[0])
                self.assertEqual((self.target / damaged).read_bytes(), self.cosign if damaged == 'cosign' else self.oras)
                self.assertEqual(self.install(offline=True).returncode, 0)

    def test_archive_and_extracted_executable_are_both_checked_before_install(self):
        for stage in ('archive', 'executable'):
            with self.subTest(stage=stage):
                self.setUp()
                (self.target / 'oras').write_bytes(b'previous corrupt oras')
                bad = archive(b'untrusted executable')
                (self.payloads / 'oras.tar.gz').write_bytes(bad)
                if stage == 'executable':
                    self.script.write_text(self.script.read_text().replace(
                        hashlib.sha256(self.tar).hexdigest(), hashlib.sha256(bad).hexdigest()))
                self.assertNotEqual(self.install().returncode, 0)
                self.assertEqual((self.target / 'oras').read_bytes(), b'previous corrupt oras')

    def test_installed_prerequisites_do_not_query_package_repositories(self):
        helper = HOST / 'updater/install-packages.sh'
        self.assertTrue(helper.is_file())
        result = subprocess.run(['bash', str(helper)], env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_prerequisites_use_only_the_existing_oracle_repositories(self):
        (self.commands / 'rpm').write_text('#!/bin/bash\nexit 1\n')
        (self.commands / 'dnf').write_text(r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
with (Path(os.environ['TOOL_TEST_ROOT']) / 'packages').open('a') as output:
    output.write(json.dumps(sys.argv[1:]) + '\n')
''')
        result = subprocess.run(['bash', str(HOST / 'updater/install-packages.sh')],
                                env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = [json.loads(line) for line in (self.root / 'packages').read_text().splitlines()]
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0][:4], ['--disablerepo=*', '--enablerepo=ol9_baseos_latest',
                                      '--enablerepo=ol9_appstream', 'install'])
        self.assertIn('podman', calls[0])
        self.assertEqual(calls[1], ['--disablerepo=*', '--enablerepo=ol9_oci_included',
                                    'install', '-y', 'python39-oci-sdk'])


if __name__ == '__main__':
    unittest.main()
