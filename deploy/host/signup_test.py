"""Keep an unset or blank signup allowlist from starting the bundle."""

from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class SignupCheckTest(unittest.TestCase):
    def test_allowlist_is_required_without_printing_its_value(self):
        cases = [
            ("SERVER_LISTEN=127.0.0.1:8080\n", False),
            ("SERVER_SIGNUP_EMAILS=\n", False),
            ("SERVER_SIGNUP_EMAILS= , \t\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid\n", True),
            ("SERVER_SIGNUP_EMAILS=owner+alias@example.invalid,other@sub.example.invalid\n", True),
            ("SERVER_SIGNUP_EMAILS=@\n", False),
            ("SERVER_SIGNUP_EMAILS=a@\n", False),
            ("SERVER_SIGNUP_EMAILS=@example.invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=a@localhost\n", False),
            ("SERVER_SIGNUP_EMAILS=a@.invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=a@example.\n", False),
            ("SERVER_SIGNUP_EMAILS=a@b..invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid,\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid,,other@example.invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid,@\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid, other@example.invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=owner @example.invalid\n", False),
            ("SERVER_SIGNUP_EMAILS=owner@example.invalid\nSERVER_SIGNUP_EMAILS=\n", False),
        ]
        with tempfile.TemporaryDirectory(prefix="trachuan-signup-test-") as work:
            env_file = Path(work) / "server.env"
            for content, accepted in cases:
                with self.subTest(content=content):
                    env_file.write_text(content)
                    result = subprocess.run(["bash", str(HOST / "check-signup.sh"), str(env_file)], text=True, capture_output=True)
                    self.assertEqual(result.returncode == 0, accepted, result.stderr)
                    self.assertNotIn("owner@example.invalid", result.stdout + result.stderr)

    def test_env_file_is_not_executed(self):
        with tempfile.TemporaryDirectory(prefix="trachuan-signup-test-") as work:
            marker = Path(work) / "executed"
            env_file = Path(work) / "server.env"
            env_file.write_text(f"SERVER_SIGNUP_EMAILS=$(touch {marker})\n")
            subprocess.run(["bash", str(HOST / "check-signup.sh"), str(env_file)], capture_output=True)
            self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
