from repositories_test_support import enable_dual_policy as setUpModule

import copy
import importlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from corpus_test_support import write_fixture


class HeadingReviewTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_manifest')
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name)
        write_fixture(self.path)
        self.reviews = self.path / 'reviews.yaml'
        self.entry = dict(release='20990102_1', projection=1, scan_sha256='a' * 64,
                          exclusions_sha256='b' * 64, reviewer='coordinator', date='2026-11-16')

    def create(self):
        return json.loads(self.module.create('dannyota/trachuan', 'a' * 40, 1, 'vn', '20990102_1', self.path,
                                            self.path / 'report.json', heading_reviews_path=self.reviews))

    def test_copy_and_omission(self):
        for data in ('', '[]', json.dumps([dict(self.entry, release='20990103_1')])):
            self.reviews.write_text(data)
            self.assertNotIn('heading_review', self.create())
        self.reviews.write_text(json.dumps([self.entry]))
        manifest = self.create()
        self.assertEqual(manifest['heading_review'], {k: v for k, v in self.entry.items() if k != 'release'})
        self.module.validate_manifest(manifest)

    def test_yaml_block_mapping_and_dates(self):
        self.reviews.write_text('- release: 20990102_1\n  projection: 1\n  scan_sha256: "' + 'a' * 64 +
                                '"\n  exclusions_sha256: "' + 'b' * 64 +
                                '"\n  reviewer: coordinator\n  date: 2026-11-16\n')
        self.assertEqual(self.create()['heading_review']['date'], '2026-11-16')

    def test_rejects_malformed_reviews(self):
        cases = ['null', '{}', '[]\n---\n[]', '[', json.dumps([self.entry, self.entry]),
                 json.dumps([self.entry]).replace('"projection": 1', '"projection": 1, "projection": 1')]
        for field, value in [('projection', 2), ('projection', 1.0), ('projection', True), ('scan_sha256', 'BAD'),
                             ('exclusions_sha256', 'BAD'), ('reviewer', 'Person Name'),
                             ('date', '2026-02-30'), ('extra', 'value'), ('release', 'bad')]:
            entry = copy.deepcopy(self.entry)
            entry[field] = value
            cases.append(json.dumps([entry]))
        for field in self.entry:
            cases.append(json.dumps([{k: v for k, v in self.entry.items() if k != field}]))
        for data in cases:
            with self.subTest(data=data):
                self.reviews.write_text(data)
                with self.assertRaises(ValueError):
                    self.create()

    def test_manifest_review_closed_schema(self):
        self.reviews.write_text(json.dumps([self.entry]))
        manifest = self.create()
        for field, value in [('projection', True), ('projection', 2), ('reviewer', 'Person Name'),
                             ('date', 'bad'), ('extra', 'value')]:
            with self.subTest(field=field, value=value):
                mutated = copy.deepcopy(manifest)
                mutated['heading_review'][field] = value
                with self.assertRaises(ValueError):
                    self.module.validate_manifest(mutated)

    def test_existing_manifest_reader_needs_only_standard_library(self):
        self.reviews.write_text('[]')
        data = self.module.create('dntmakers/trachuan', 'a' * 40, 1, 'vn', '20990102_1', self.path,
                                  self.path / 'report.json', heading_reviews_path=self.reviews)
        script = 'import sys; sys.path.insert(0, sys.argv[1]); from corpus_manifest import read_manifest; read_manifest(sys.stdin.buffer.read())'
        result = subprocess.run([sys.executable, '-S', '-c', script, str(Path(__file__).parent)],
                                input=data, capture_output=True, check=False)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
