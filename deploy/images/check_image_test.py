"""Reject unexpected payloads in a release filesystem."""

import importlib.util
import io
import pathlib
import tarfile
import unittest

SPEC = importlib.util.spec_from_file_location("check_image", pathlib.Path(__file__).with_name("check-image.py"))


class InventoryTest(unittest.TestCase):
    def test_unexpected_binary_is_rejected(self):
        self.assertRaises(ValueError, self.scan, "app/fakeembed", b"\x7fELFpayload")

    def test_unexpected_native_archive_is_rejected(self):
        self.assertRaises(ValueError, self.scan, "app/unexpected.a", b"!<arch>\narchive")

    def test_weights_are_rejected_even_without_execute_bit(self):
        self.assertRaises(ValueError, self.scan, "models/onnx/model_int8.onnx", b"weights")

    def test_allowed_production_binary_is_listed(self):
        self.assertEqual(set(self.scan("app/server", b"\x7fELFpayload")), {"app/server"})

    def test_changed_base_binary_is_rejected(self):
        self.assertRaises(ValueError, self.scan, "lib/loader", b"\x7fELFchanged", {"lib/loader": "old-hash"})

    def scan(self, name, data, baseline=None):
        module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(module)
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w") as archive:
            entry = tarfile.TarInfo(name)
            entry.size = len(data)
            entry.mode = 0o644
            archive.addfile(entry, io.BytesIO(data))
        buffer.seek(0)
        return module.inventory(buffer, baseline or {}, {"app/server"})


if __name__ == "__main__":
    unittest.main()
