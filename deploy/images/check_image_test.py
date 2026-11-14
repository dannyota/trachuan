from contextlib import redirect_stdout
import importlib.util
import io
import pathlib
import re
import tarfile
import unittest
from unittest.mock import patch

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

    def test_postgres_accepts_all_four_observed_additions(self):
        self.assertEqual(set(self.scan_postgres()), {
            "usr/bin/pgbackrest",
            "usr/lib/aarch64-linux-gnu/libssh2.so.1.0.1",
            "var/lib/dpkg/info/pgbackrest.postinst",
            "var/lib/dpkg/info/pgbackrest.postrm",
        })

    def test_postgres_rejects_unobserved_libraries_scripts_and_binaries(self):
        for name in ["usr/lib/aarch64-linux-gnu/libyaml-0.so.2.0.9",
                     "usr/lib/aarch64-linux-gnu/libssh2.so.1.0.2",
                     "var/lib/dpkg/info/pgbackrest.preinst",
                     "var/lib/dpkg/info/pgbackrest.postinst.extra", "usr/bin/curl"]:
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, "unexpected binary: " + re.escape(name) + "$"):
                self.scan_postgres(name)

    def scan_postgres(self, extra=None):
        module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(module)
        payloads = {
            "usr/bin/pgbackrest": b"\x7fELFbackup",
            "usr/lib/aarch64-linux-gnu/libssh2.so.1.0.1": b"\x7fELFssh",
            "var/lib/dpkg/info/pgbackrest.postinst": b"#!/bin/sh\nexit 0\n",
            "var/lib/dpkg/info/pgbackrest.postrm": b"#!/bin/sh\nexit 0\n",
        }
        if extra:
            payloads[extra] = b"#!/bin/sh\nexit 0\n"
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w") as archive:
            for name, payload in payloads.items():
                entry = tarfile.TarInfo(name)
                entry.size = len(payload)
                archive.addfile(entry, io.BytesIO(payload))
        result = {}
        def image_inventory(image, baseline=None, allowed=None):
            if baseline is None:
                return {}
            buffer.seek(0)
            result.update(module.inventory(buffer, baseline, allowed))
            return result
        with patch.object(module.sys, "argv", ["check-image.py", "local-postgres", "postgres"]), \
             patch.object(module.subprocess, "run"), \
             patch.object(module, "image_inventory", side_effect=image_inventory), \
             redirect_stdout(io.StringIO()):
            module.main()
        return result

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
