from pathlib import Path
import unittest

HOST = Path(__file__).resolve().parent


class MetadataRuleTest(unittest.TestCase):
    def test_installer_blocks_metadata_for_the_trachuan_uid_that_runs_pasta(self):
        script = (HOST / 'install.sh').read_text()
        self.assertIn('for owner in "$uid" "$subuid"; do', script)
        self.assertIn('rule=(ipv4 filter OUTPUT 0 -d 169.254.169.254 -p tcp --dport 80 -m owner --uid-owner "$owner" -j REJECT)', script)
        self.assertNotIn('backup-metadata-rule.sh', script)


if __name__ == '__main__':
    unittest.main()
