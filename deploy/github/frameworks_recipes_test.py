import base64
from decimal import Decimal
import hashlib
import importlib
import json
from pathlib import Path
import tomllib
import unittest

VECTORS = Path(__file__).with_name('testdata') / 'frameworks-recipe-vectors.json'
ROOT = Path(__file__).resolve().parents[2]


def one_field(line):
    decoder = json.JSONDecoder()
    text = line.strip().removesuffix(',')
    if text in ('{', '}', '[', ']', '{}', '[]'):
        return True
    first, end = decoder.raw_decode(text)
    rest = text[end:]
    if not rest:
        return not isinstance(first, (dict, list))
    if not isinstance(first, str) or not rest.startswith(': '):
        return False
    rest = rest[2:]
    if rest in ('{', '[', '{}', '[]'):
        return True
    value, end = decoder.raw_decode(rest)
    return end == len(rest) and not isinstance(value, (dict, list))


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
                normalized = base64.b64decode(vector['normalized_base64'], validate=True)
                self.assertEqual(self.module.go_json(vector['value']).encode(), normalized)
                self.assertEqual(hashlib.sha256(normalized).hexdigest(), vector['sha256'])
                self.assertEqual(self.module.recipe_sha256(vector['value']), vector['sha256'])

    def test_hash_survives_the_jsonb_text_form_of_release_json(self):
        for vector in self.vectors:
            if vector['name'] == 'negative_zero':
                continue
            with self.subTest(name=vector['name']):
                row = self.module.decode_floats(jsonb_text(vector['value']).encode())
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

    def test_vector_file_holds_one_field_per_line(self):
        lines = VECTORS.read_text().rstrip('\n').split('\n')
        self.assertGreater(len(lines), 100)
        for number, line in enumerate(lines, 1):
            with self.subTest(number=number):
                self.assertTrue(one_field(line))

    def test_secret_allowlist_keeps_the_exact_field_patterns(self):
        config = tomllib.loads((ROOT / '.gitleaks.toml').read_text())
        entry = next(item for item in config['allowlists'] if item['description'] == 'Model file hashes in embedder recipes')
        self.assertIn(r'^deploy/github/testdata/frameworks-recipe-vectors\.json$', entry['paths'])
        self.assertEqual(entry['regexes'], [r'(tokenizer|weights)_sha256"\s*:\s*"[0-9a-f]{64}"',
                                            r'(resolved|tokenizer)_revision"\s*:\s*"[0-9a-f]{40}"'])

    def test_line_separators_are_written_as_escapes_in_the_source(self):
        source = Path(self.module.__file__).read_bytes()
        self.assertNotIn('\u2028'.encode(), source)
        self.assertNotIn('\u2029'.encode(), source)
        self.assertEqual(self.module.go_string('\u2028\u2029'), '"\\u2028\\u2029"')

    def test_plain_decode_keeps_integers_for_the_manifest(self):
        self.assertIs(type(self.module.decode(b'{"version":1}')['version']), int)
        self.assertIs(type(self.module.decode_floats(b'{"version":1}')['version']), float)


if __name__ == '__main__':
    unittest.main()
