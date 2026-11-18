import hashlib
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

    def expected(self):
        data = self.path.read_bytes()
        return hashlib.sha256(data).hexdigest(), len(data)

    def test_law_upload_without_expected_values_sends_the_file_itself(self):
        calls = []

        def client(args):
            calls.append(args)
            return {'ETag': '"own-upload"'}

        self.module.upload_file(self.path, 'vn/20990102_1/corpus.dump', client)
        self.assertEqual(calls[0][calls[0].index('--body') + 1], str(self.path))

    def test_expected_small_upload_sends_a_checked_private_copy(self):
        bodies = []

        def client(args):
            body = Path(args[args.index('--body') + 1])
            bodies.append((body, body.read_bytes()))
            return {'ETag': '"own-upload"'}

        result = self.module.upload_file(self.path, 'frameworks/20990102/report.json', client, expected=self.expected())
        self.assertEqual(result, {'ContentLength': 7, 'ETag': '"own-upload"'})
        self.assertNotEqual(bodies[0][0], self.path)
        self.assertEqual(bodies[0][1], b'abcdefg')
        self.assertFalse(bodies[0][0].exists())

    def test_changed_small_file_is_never_sent(self):
        digest, size = self.expected()
        for expected in (('0' * 64, size), (digest, size + 1)):
            calls = []
            with self.subTest(expected=expected), self.assertRaisesRegex(ValueError, 'report.json'):
                self.module.upload_file(self.path, 'frameworks/20990102/report.json', calls.append, expected=expected)
            self.assertEqual(calls, [])

    def multipart_client(self, calls):
        def client(args):
            calls.append(args)
            if 'create-multipart-upload' in args:
                return {'UploadId': 'synthetic-upload'}
            return {'ETag': '"part"'}
        return client

    def test_expected_multipart_upload_completes_when_the_stream_matches(self):
        calls = []
        result = self.module.upload_file(self.path, 'frameworks/20990102/frameworks.dump', self.multipart_client(calls),
                                         part_size=3, expected=self.expected())
        self.assertEqual(result['ContentLength'], 7)
        self.assertTrue(any('complete-multipart-upload' in call for call in calls))

    def test_changed_size_starts_no_multipart_upload(self):
        calls = []
        with self.assertRaisesRegex(ValueError, 'frameworks.dump'):
            self.module.upload_file(self.path, 'frameworks/20990102/frameworks.dump', self.multipart_client(calls),
                                    part_size=3, expected=(self.expected()[0], 8))
        self.assertEqual(calls, [])

    def test_changed_multipart_stream_aborts_before_completion(self):
        calls = []
        with self.assertRaisesRegex(ValueError, 'frameworks.dump'):
            self.module.upload_file(self.path, 'frameworks/20990102/frameworks.dump', self.multipart_client(calls),
                                    part_size=3, expected=('0' * 64, 7))
        self.assertIn('abort-multipart-upload', calls[-1])
        self.assertFalse(any('complete-multipart-upload' in call for call in calls))

    def test_bucket_defaults_to_law_and_can_be_named(self):
        calls = []

        def client(args):
            calls.append(args)
            return {'ETag': '"own-upload"'}

        self.module.upload_file(self.path, 'vn/20990102_1/corpus.dump', client)
        self.module.upload_file(self.path, 'frameworks/20990102/frameworks.dump', client, bucket='trachuan-frameworks-releases')
        self.assertEqual([args[args.index('--bucket') + 1] for args in calls],
                         ['trachuan-releases', 'trachuan-frameworks-releases'])

    def test_receipts_compare_size_and_etag_in_the_named_bucket(self):
        requests = []

        def client(args):
            requests.append(args)
            return {'ContentLength': 7, 'ETag': '"own"'}

        receipts = {'frameworks/20990102/report.json': {'ContentLength': 7, 'ETag': '"own"'}}
        self.module.verify_receipts(receipts, client, bucket='trachuan-frameworks-releases')
        self.assertEqual(requests, [['head-object', '--bucket', 'trachuan-frameworks-releases',
                                     '--key', 'frameworks/20990102/report.json']])
        for change in ({'ContentLength': 8}, {'ETag': '"other"'}):
            with self.subTest(change=change), self.assertRaisesRegex(ValueError, 'report.json'):
                self.module.verify_receipts({key: value | change for key, value in receipts.items()}, client)

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
