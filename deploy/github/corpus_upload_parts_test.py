import importlib
import json
from pathlib import Path
import tempfile
import unittest


class CorpusUploadPartsTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_upload')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / 'corpus.dump'
        self.path.write_bytes(b'abcdefg')

    def test_small_upload_uses_its_returned_etag_and_local_size(self):
        calls = []

        def client(args):
            calls.append(args)
            return {'ETag': '"own-upload"'}

        result = self.module.upload_file(self.path, 'vn/20990102_1/corpus.dump', client)
        self.assertEqual(result, {'ContentLength': 7, 'ETag': '"own-upload"'})
        self.assertIn('put-object', calls[0])
        self.assertIn('--content-length', calls[0])

    def test_multipart_uses_completion_etag_and_orders_every_uploaded_part(self):
        calls, data = [], []

        def client(args):
            calls.append(args)
            if 'create-multipart-upload' in args:
                return {'UploadId': 'synthetic-upload'}
            if 'upload-part' in args:
                data.append(Path(args[args.index('--body') + 1]).read_bytes())
                return {'ETag': f'"part-{len(data)}"'}
            if 'complete-multipart-upload' in args:
                parts = json.loads(args[args.index('--multipart-upload') + 1])['Parts']
                self.assertEqual(parts, [dict(PartNumber=n, ETag=f'"part-{n}"') for n in range(1, 4)])
                return {'ETag': '"own-multipart"'}
            self.fail(str(args))

        result = self.module.upload_file(self.path, 'vn/20990102_1/corpus.dump', client, part_size=3)
        self.assertEqual(b''.join(data), self.path.read_bytes())
        self.assertEqual(result, {'ContentLength': 7, 'ETag': '"own-multipart"'})
        self.assertEqual(len(calls), 5)

    def test_failed_part_aborts_without_completing_the_upload(self):
        calls = []

        def client(args):
            calls.append(args)
            if 'create-multipart-upload' in args:
                return {'UploadId': 'synthetic-upload'}
            if 'upload-part' in args:
                raise ValueError('synthetic failed part')
            return {}

        with self.assertRaisesRegex(ValueError, 'failed part'):
            self.module.upload_file(self.path, 'vn/20990102_1/corpus.dump', client, part_size=3)
        self.assertIn('abort-multipart-upload', calls[-1])
        self.assertFalse(any('complete-multipart-upload' in call for call in calls))


if __name__ == '__main__':
    unittest.main()
