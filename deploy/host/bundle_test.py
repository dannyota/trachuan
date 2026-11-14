import configparser
import json
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


def objects(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from objects(child)


class InstallerTest(unittest.TestCase):
    def test_cgroup_template_matches_podman_field(self):
        templates = re.findall(r"podman info --format '([^']+)'", (HOST / "install.sh").read_text())
        self.assertEqual(templates, ["{{.Host.CgroupsVersion}}"])


class ListenerTest(unittest.TestCase):
    def test_rendered_services_use_host_network_without_port_forwarding(self):
        with tempfile.TemporaryDirectory(prefix="trachuan-host-test-") as work:
            work = Path(work)
            repositories = {
                "SERVER": "ghcr.io/dannyota/trachuan-server",
                "WEB": "ghcr.io/dannyota/trachuan-web",
                "QUERYEMBED": "ghcr.io/dannyota/trachuan-queryembed",
                "CADDY": "docker.io/library/caddy",
                "POSTGRES": "docker.io/pgvector/pgvector",
            }
            pins = work / "images.env"
            pins.write_text("".join(
                f"{name}_IMAGE={repo}@sha256:{'1' * 64}\n"
                for name, repo in repositories.items()
            ))
            (work / "units").mkdir()
            for name in ["trachuan.network", "trachuan-db.network"]:
                (work / "units" / name).write_text("[Network]\n")
            subprocess.run(["bash", str(HOST / "render-units.sh"), str(pins), str(work / "units")], check=True)
            units = list((work / "units").glob("*.container"))
            self.assertEqual(len(units), 6)
            self.assertEqual(list((work / "units").glob("*.network")), [])
            self.assertFalse((work / "units" / "trachuan-postgres.volume").exists())
            for unit in units:
                config = configparser.ConfigParser(strict=False, interpolation=None)
                config.read(unit)
                with self.subTest(unit=unit.name):
                    container = config["Container"]
                    self.assertEqual(container["Network"], "host")
                    self.assertNotIn("PublishPort", container)
                    self.assertNotIn("IP", container)
                    if unit.name == "trachuan-postgres.container":
                        self.assertEqual(config["Unit"]["RequiresMountsFor"], "/srv/trachuan/data")
                        self.assertEqual(config["Service"]["ExecStartPre"], "/usr/bin/mountpoint -q /srv/trachuan/data")
                        self.assertIn("Volume=/srv/trachuan/data/postgres:/var/lib/postgresql:Z,U",
                                      unit.read_text().splitlines())
                        self.assertIn("listen_addresses=127.0.0.1", shlex.split(container["Exec"]))
                        environment = {}
                        for line in unit.read_text().splitlines():
                            if line.startswith("Environment="):
                                for assignment in shlex.split(line.split("=", 1)[1]):
                                    name, value = assignment.split("=", 1)
                                    environment[name] = value
                        with self.subTest(policy="authentication"):
                            self.assertEqual(environment.get("POSTGRES_HOST_AUTH_METHOD"), "scram-sha-256")
                            self.assertEqual(set(shlex.split(environment.get("POSTGRES_INITDB_ARGS", ""))), {
                                "--auth-host=scram-sha-256", "--auth-local=scram-sha-256",
                            })
                        with self.subTest(policy="shared memory"):
                            self.assertIn("--shm-size=3g", shlex.split(container["PodmanArgs"]))
                            self.assertIn("--memory=8g", shlex.split(container["PodmanArgs"]))
                    hooks = [shlex.split(line.split("=", 1)[1]) for line in unit.read_text().splitlines()
                             if line.startswith("ExecStartPre=")]
                    if unit.name == "trachuan-server.container":
                        self.assertIn(["/srv/trachuan/host/check-signup.sh", "/srv/trachuan/host/server.env"], hooks)
                    if unit.name == "trachuan-migrate.container":
                        self.assertIn(["/srv/trachuan/host/check-postgres-auth.sh"], hooks)
                    if unit.name == "trachuan-queryembed.container":
                        args = shlex.split(container["Exec"])
                        self.assertEqual(args[args.index("--listen") + 1], "127.0.0.1:8091")


@unittest.skipUnless(shutil.which("caddy"), "native Caddy is unavailable")
class EdgeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.config = json.loads(subprocess.check_output([
            "caddy", "adapt", "--config", str(HOST / "Caddyfile"), "--adapter", "caddyfile",
        ], text=True))

    def test_default_configuration_does_not_log_client_addresses(self):
        self.assertNotIn("client_address_check", self.config.get("logging", {}).get("logs", {}))

    def test_upstreams_are_loopback(self):
        upstreams = {upstream["dial"] for obj in objects(self.config)
                     if obj.get("handler") == "reverse_proxy" for upstream in obj["upstreams"]}
        self.assertEqual(upstreams, {"127.0.0.1:8080", "127.0.0.1:3000"})

    def test_only_api_oauth_and_mcp_paths_go_to_server(self):
        paths = [obj["path"] for obj in objects(self.config)
                 if "path" in obj and "/api/v1/*" in obj["path"]]
        self.assertEqual(len(paths), 1)
        self.assertEqual(set(paths[0]), {"/api/v1/*", "/mcp", "/mcp/*", "/oauth/*", "/.well-known/oauth-*"})

    def test_health_listener_is_loopback(self):
        listeners = [listener for obj in objects(self.config) for listener in obj.get("listen", [])]
        self.assertIn("0.0.0.0:443", listeners)
        self.assertIn("127.0.0.1:8081", listeners)
        self.assertNotIn(":8081", listeners)


if __name__ == "__main__":
    unittest.main()
