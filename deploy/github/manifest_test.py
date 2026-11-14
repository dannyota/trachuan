import copy
import hashlib
import importlib
import json
from pathlib import Path
import unittest


class ManifestTest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(Path(__file__).with_name('manifest.py').exists(), 'manifest builder missing')
        self.m = importlib.import_module('manifest')
        self.args = dict(commit='a' * 40, ref='refs/tags/v1.2.3', sequence=12,
                         images={name: 'sha256:' + 'b' * 64
                                 for name in ('server', 'web', 'queryembed', 'postgres')},
                         caddy='docker.io/library/caddy@sha256:' + 'c' * 64,
                         bundle=b'host bundle')

    def test_release_canonical_bytes_and_exact_fields(self):
        result = self.m.release(**self.args)
        expected = ('{"commit":"' + 'a' * 40 + '","corpus_release":null,"host_bundle":'
                    '{"sha256":"' + hashlib.sha256(b'host bundle').hexdigest() + '","size":11},'
                    '"images":{"postgres":"ghcr.io/dannyota/trachuan-postgres@sha256:' + 'b' * 64 + '",'
                    '"queryembed":"ghcr.io/dannyota/trachuan-queryembed@sha256:' + 'b' * 64 + '",'
                    '"server":"ghcr.io/dannyota/trachuan-server@sha256:' + 'b' * 64 + '",'
                    '"web":"ghcr.io/dannyota/trachuan-web@sha256:' + 'b' * 64 + '"},'
                    '"kind":"trachuan.image-release","ref":"refs/tags/v1.2.3",'
                    '"release":"v1.2.3","repository":"dannyota/trachuan","rollback_target":null,'
                    '"sequence":12,"target":"production","upstream":{"caddy":'
                    '"docker.io/library/caddy@sha256:' + 'c' * 64 + '"},"version":1}').encode()
        self.assertEqual(result, expected)
        self.assertEqual(self.m.canonical({'text': 'Việt'}), b'{"text":"Vi\xe1\xbb\x87t"}')

    def test_rejects_bad_image_or_missing_image(self):
        for value in ('latest', 'sha256:abc', 'sha256:' + 'B' * 64,
                      'evil.example/server@sha256:' + 'b' * 64):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.m.release(**(self.args | {'images': self.args['images'] | {'server': value}}))
        for images in ({}, self.args['images'] | {'extra': 'sha256:' + 'b' * 64}):
            with self.assertRaises(ValueError):
                self.m.release(**(self.args | {'images': images}))

    def test_rejects_invalid_release_fields(self):
        for key, value in [('ref', 'refs/heads/main'), ('ref', 'refs/tags/v1.2.3-rc1'),
                           ('ref', 'refs/tags/v1.2.3\n'), ('commit', 'a' * 39),
                           ('sequence', 0), ('sequence', True),
                           ('caddy', 'docker.io/library/caddy:latest'),
                           ('caddy', 'evil.example/caddy@sha256:' + 'c' * 64)]:
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                self.m.release(**(self.args | {key: value}))

    def test_caddy_env_requires_one_digest_reference(self):
        self.assertEqual(self.m.caddy_from_env('SERVER_IMAGE=unused\nCADDY_IMAGE=' + self.args['caddy']),
                         self.args['caddy'])
        for text in ('', 'CADDY_IMAGE=caddy:latest',
                     ('CADDY_IMAGE=' + self.args['caddy'] + '\n') * 2):
            with self.assertRaises(ValueError):
                self.m.caddy_from_env(text)

    def test_validation_rejects_wrong_contract_or_noncanonical_bytes(self):
        data = self.m.release(**self.args)
        self.assertEqual(self.m.read_release(data)['sequence'], 12)
        for key, value in [('kind', 'other'), ('version', True), ('repository', 'other/repo'),
                           ('target', 'other'), ('rollback_target', 'b' * 64),
                           ('images', {'server': 'unlisted'}), ('extra', 1)]:
            doc = json.loads(data)
            doc[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.m.read_release(self.m.canonical(doc))
        with self.assertRaises(ValueError):
            self.m.read_release(data + b'\n')

    def test_rollback_canonical_document_and_constraints(self):
        current = json.loads(self.m.release(**self.args))
        target = copy.deepcopy(current)
        target['sequence'] = 11
        result = self.m.rollback('a' * 64, 'b' * 64, 7, current, target)
        self.assertEqual(result, ('{"current":"' + 'a' * 64 + '","kind":"trachuan.rollback",'
                                 '"sequence":7,"target":"' + 'b' * 64 + '","version":1}').encode())
        for bad in ('B' * 64, 'a' * 63, 'x;echo unsafe'):
            with self.assertRaises(ValueError):
                self.m.rollback(bad, 'b' * 64, 7, current, target)
        for sequence in (12, 13):
            with self.assertRaises(ValueError):
                self.m.rollback('a' * 64, 'b' * 64, 7, current, target | {'sequence': sequence})
        with self.assertRaises(ValueError):
            self.m.rollback('a' * 64, 'b' * 64, 7, current, target | {'rollback_target': 'c' * 64})


if __name__ == '__main__':
    unittest.main()
