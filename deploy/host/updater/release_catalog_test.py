import unittest

from updater import deploy_frameworks_check as frameworks
from updater import deploy_release_catalog as catalog
from updater.corpus_verify_test import manifest
from updater.frameworks_check_test import UNQUALIFIED


class LawCatalogTest(unittest.TestCase):
    def setUp(self):
        self.sql = catalog.law(manifest()['extensions'])

    def test_frameworks_and_law_compare_the_same_snapshot(self):
        self.assertIs(frameworks.SNAPSHOT, catalog.SNAPSHOT)
        self.assertIs(frameworks.KINDS, catalog.KINDS)
        self.assertIn(catalog.SNAPSHOT, self.sql)

    def test_only_the_manifest_extensions_are_expected_in_public_and_owned_by_a_superuser(self):
        self.assertIn("('extension','ltree','1.3 public superuser')", self.sql)
        self.assertIn("('extension','vector','0.8.1 public superuser')", self.sql)
        self.assertEqual(self.sql.count("('extension',"), 2)
        self.assertNotIn("('setting',", self.sql)

    def test_pg_prewarm_is_optional_but_must_be_in_public_and_owned_by_a_superuser(self):
        self.assertIn("f.kind='extension' AND f.identity='pg_prewarm' AND f.definition LIKE '% public superuser'", self.sql)
        self.assertNotIn("('extension','pg_prewarm'", self.sql)

    def test_every_catalog_name_is_qualified_and_only_the_first_kind_is_returned(self):
        self.assertEqual(UNQUALIFIED.findall(self.sql), [])
        self.assertTrue(self.sql.endswith("SELECT coalesce((SELECT kind FROM differences ORDER BY kind LIMIT 1),'ok');\n"))
        self.assertNotIn('identity FROM differences', self.sql)

    def test_extension_names_must_be_plain(self):
        with self.assertRaises(ValueError):
            catalog.law({'vector; DROP': '1'})


if __name__ == '__main__':
    unittest.main()
