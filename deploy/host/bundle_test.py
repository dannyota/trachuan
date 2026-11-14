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
    def test_metrics_bundle_and_metadata_firewall(self):
        script = (HOST / "install.sh").read_text()
        for path in ("metrics/publish-metrics.py", "systemd/trachuan-metrics.service", "systemd/trachuan-metrics.timer"):
            self.assertTrue((HOST / path).is_file(), path)
        for value in ("ol9_oci_included", "python39-oci-sdk", "--uid-owner", "169.254.169.254", "--dport 80", "--query-rule", "--add-rule", "trachuan-metrics.timer"):
            self.assertIn(value, script)

    def test_restore_directory_is_provisioned_before_rootless_use(self):
        source = (HOST / "install.sh").read_text()
        loop = re.search(r"for dir in ([^;]+); do\n(.*?)\ndone", source, re.S)
        self.assertIn("/srv/trachuan/data/restore-check", shlex.split(loop.group(1)))
        self.assertIn('install -d -o trachuan -g trachuan -m 0700 "$dir"', loop.group(2))

    def test_cgroup_template_matches_podman_field(self):
        templates = re.findall(r"podman info --format '([^']+)'", (HOST / "install.sh").read_text())
        self.assertEqual(templates, ["{{.Host.CgroupsVersion}}"])


class ServerEnvironmentTest(unittest.TestCase):
    def test_account_key_file_and_smtp_settings(self):
        lines = (HOST / "server.env").read_text().splitlines()
        expected = {
            "SERVER_ACCOUNT_KEYS_FILE": "/run/secrets/account_keys",
            "SERVER_SMTP_HOST": "smtp.email.ap-singapore-1.oci.oraclecloud.com",
            "SERVER_SMTP_PORT": "587",
            "SERVER_MAIL_FROM": "no-reply@trachuan.vn",
        }
        for name, value in expected.items():
            with self.subTest(variable=name):
                self.assertEqual([line for line in lines if line.startswith(f"{name}=")],
                                 [f"{name}={value}"])


class ListenerTest(unittest.TestCase):
    def test_rendered_services_use_host_network_without_port_forwarding(self):
        with tempfile.TemporaryDirectory(prefix="trachuan-host-test-") as work:
            work = Path(work)
            repositories = {
                "SERVER": "ghcr.io/dannyota/trachuan-server",
                "WEB": "ghcr.io/dannyota/trachuan-web",
                "QUERYEMBED": "ghcr.io/dannyota/trachuan-queryembed",
                "CADDY": "docker.io/library/caddy",
                "POSTGRES": "ghcr.io/dannyota/trachuan-postgres",
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
            schedules = {"full": "Sun *-*-* 02:00:00", "diff": "Mon..Sat *-*-* 02:00:00",
                         "verify": "Wed *-*-* 04:00:00", "health": "*-*-* *:00:00",
                         "restore-check": "*-*-01 05:00:00"}
            for kind, schedule in schedules.items():
                timer = configparser.ConfigParser(interpolation=None)
                timer.read(work / "units" / f"trachuan-backup-{kind}.timer")
                self.assertEqual(timer["Timer"]["OnCalendar"], schedule+" Asia/Ho_Chi_Minh")
                self.assertEqual(timer["Timer"]["Persistent"], "true")
                service = configparser.ConfigParser(interpolation=None)
                service.read(work / "units" / f"trachuan-backup-{kind}.service")
                self.assertIn(f"/srv/trachuan/host/backup.py {kind}", service["Service"]["ExecStart"])
                self.assertEqual(service["Service"]["UMask"], "0027")
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
                        args = shlex.split(container["Exec"])
                        self.assertIn("archive_mode=on", args)
                        self.assertIn("archive_timeout=300", args)
                        self.assertIn("archive_command=pgbackrest --stanza=trachuan archive-push %%p", args)
                        self.assertEqual(container["Entrypoint"], "/usr/local/bin/pgbackrest-entrypoint.sh")
                        text = unit.read_text()
                        self.assertIn("Tmpfs=/run/pgbackrest:rw,noexec,nosuid,nodev,uid=999,gid=999,mode=0700", text)
                        self.assertIn("Volume=/srv/trachuan/data/pgbackrest:/srv/trachuan/data/pgbackrest:Z,U", text)
                        for secret in ["backup_s3_key", "backup_s3_key_secret", "postgres_password"]:
                            self.assertIn(f"Secret={secret},type=mount,uid=999,gid=999,mode=0400", text)
                        environment = {}
                        for line in unit.read_text().splitlines():
                            if line.startswith("Environment="):
                                for assignment in shlex.split(line.split("=", 1)[1]):
                                    name, value = assignment.split("=", 1)
                                    environment[name] = value
                        self.assertEqual(environment.get("PGBACKREST_CONFIG_INCLUDE_PATH"), "/run/pgbackrest/conf.d")
                        self.assertEqual(environment.get("PGPASSFILE"), "/run/pgbackrest/pgpass")
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
                        self.assertEqual(container["User"], "65532:65532")
                        secrets = {
                            "account_keys": "type=mount,target=/run/secrets/account_keys,uid=65532,gid=65532,mode=0400",
                            "smtp_username": "type=env,target=SERVER_SMTP_USERNAME",
                            "smtp_password": "type=env,target=SERVER_SMTP_PASSWORD",
                        }
                        for name, options in secrets.items():
                            with self.subTest(secret=name):
                                self.assertEqual([
                                    line for line in unit.read_text().splitlines()
                                    if line.startswith(f"Secret={name},")
                                ], [f"Secret={name},{options}"])
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
