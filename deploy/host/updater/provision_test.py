from pathlib import Path
import unittest

from updater import activation_test as fixture


class ProvisionTest(fixture.ActivationFixture, unittest.TestCase):
    def test_images_provision_after_postgres_and_before_migrate(self):
        self.engine.activate(self.new)
        args = [call for call, _ in self.calls]
        provision = next(i for i, a in enumerate(args) if '/usr/local/bin/provision-activator.sh' in a)
        migration = next(i for i, a in enumerate(args) if 'trachuan-migrate.service' in a and 'start' in a)
        self.assertLess(provision, migration)

    def test_postgres_mounts_secret_and_provision_script(self):
        root = Path(__file__).parents[1]
        unit = (root/'quadlet/trachuan-postgres.container').read_text()
        self.assertIn('Secret=activator_db_password,type=mount,uid=999,gid=999,mode=0400', unit)
        self.assertIn('provision-activator.sh:/usr/local/bin/provision-activator.sh:ro,z', unit)
        script = (root/'provision-activator.sh').read_text()
        self.assertIn('log_min_error_statement=panic', script)
        self.assertIn('NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS', script)
