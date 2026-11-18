from decimal import Decimal
import hashlib
import importlib
import json
from pathlib import Path
import unittest

VECTORS = Path(__file__).with_name('testdata') / 'frameworks-recipe-vectors.json'


def jsonb_text(value):
    if isinstance(value, dict):
        keys = sorted(value, key=lambda key: (len(key.encode()), key.encode()))
        return '{' + ', '.join(json.dumps(key, ensure_ascii=False) + ': ' + jsonb_text(value[key]) for key in keys) + '}'
    if isinstance(value, list):
        return '[' + ', '.join(jsonb_text(item) for item in value) + ']'
    if isinstance(value, bool) or value is None or isinstance(value, str):
        return json.dumps(value, ensure_ascii=False)
    return format(Decimal(repr(value)), 'f')


class RecipeHashTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_recipes')
        self.vectors = self.module.decode_floats(VECTORS.read_bytes())['vectors']

    def test_matches_go_recipe_sha256_byte_for_byte(self):
        self.assertGreaterEqual(len(self.vectors), 9)
        for vector in self.vectors:
            with self.subTest(name=vector['name']):
                self.assertEqual(self.module.go_json(vector['input']).encode(), vector['normalized'].encode())
                self.assertEqual(hashlib.sha256(vector['normalized'].encode()).hexdigest(), vector['sha256'])
                self.assertEqual(self.module.recipe_sha256(vector['input']), vector['sha256'])

    def test_hash_survives_the_jsonb_text_form_of_release_json(self):
        for vector in self.vectors:
            if vector['name'] == 'negative_zero':
                continue
            with self.subTest(name=vector['name']):
                row = self.module.decode_floats(jsonb_text(vector['input']).encode())
                self.assertEqual(self.module.recipe_sha256(row), vector['sha256'])

    def test_jsonb_negative_zero_fails_closed(self):
        vector = next(item for item in self.vectors if item['name'] == 'negative_zero')
        row = self.module.decode_floats(b'{"value": 0}')
        self.assertNotEqual(self.module.recipe_sha256(row), vector['sha256'])

    def test_refuses_what_go_cannot_hash(self):
        for data in (b'{"a":1e999}', b'{"a":NaN}', b'{"a":Infinity}', b'{"a":1,"a":2}', b'{"a":"\\ud800"}',
                     b'\xff{}', b'\xef\xbb\xbf{}', b'[' * 200000):
            with self.subTest(data=data[:12]), self.assertRaises(ValueError):
                self.module.recipe_sha256(self.module.decode_floats(data))
        for value in ([], None, 'text', 1.0):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.module.recipe_sha256(value)

    def test_plain_decode_keeps_integers_for_the_manifest(self):
        self.assertIs(type(self.module.decode(b'{"version":1}')['version']), int)
        self.assertIs(type(self.module.decode_floats(b'{"version":1}')['version']), float)


if __name__ == '__main__':
    unittest.main()
