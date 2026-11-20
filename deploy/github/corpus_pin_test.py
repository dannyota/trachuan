import importlib
import unittest
from unittest.mock import patch

REL = '20990102_1'


class CorpusPinTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_pin')

    def test_a_release_pin_needs_no_registry_lookup(self):
        with patch('corpus_pin.existing_release') as existing:
            self.module.require_pinned_or_signed('vn', REL, 'b' * 64, 'dannyota/trachuan')
        existing.assert_not_called()

    def test_empty_pin_needs_a_signed_release(self):
        with patch('corpus_pin.existing_release', return_value=None) as existing, \
                self.assertRaisesRegex(ValueError, 'empty release SHA-256 signs only an already signed vn-20990102_1'):
            self.module.require_pinned_or_signed('vn', REL, '', 'dntmakers/trachuan')
        self.assertEqual(existing.call_args.args[:3], ('dntmakers/trachuan', 'vn', REL))
        with patch('corpus_pin.existing_release', return_value=dict(rel=REL)):
            self.module.require_pinned_or_signed('vn', REL, '', 'dannyota/trachuan')

    def test_malformed_input_never_reaches_the_registry(self):
        for args in (('vn', REL, 'B' * 64), ('vn', REL, 'b' * 63), ('vn', REL, None), ('vn', REL, 'b' * 64 + '\n'),
                     ('VN', REL, ''), ('vn', '20990102', ''), ('../', REL, '')):
            with self.subTest(args=args), patch('corpus_pin.existing_release') as existing, self.assertRaises(ValueError):
                self.module.require_pinned_or_signed(*args, 'dannyota/trachuan')
            existing.assert_not_called()


if __name__ == '__main__':
    unittest.main()
