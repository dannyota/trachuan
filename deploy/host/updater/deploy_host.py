import re

from .deploy_io import atomic, command, CommandError, remove, Result
from .deploy_transfer import program
from .deploy_verify import as_user, pull_image


def pins(manifest):
    images = dict(manifest['images'], **manifest['upstream'])
    return ''.join(name.upper() + '_IMAGE=' + image + '\n' for name, image in sorted(images.items())).encode()


class Host:
    def __init__(self, paths, runner=command, user_prefix=None):
        self.paths, self.runner = paths, runner
        self.user_prefix = as_user() if user_prefix is None else user_prefix

    def user(self, *args, **kwargs):
        return self.runner(self.user_prefix + list(args), **kwargs)

    def systemctl(self, action, *names):
        return self.user('systemctl', '--user', action, *['trachuan-' + name + '.service' for name in names])

    def hold(self):
        atomic(self.paths.hold, b'deployment held\n', 0o644)

    def clear_hold(self):
        remove(self.paths.hold)

    def warning(self, step, message):
        Result(self.paths.result, self.paths.log).append(dict(level='warning', step=step, message=message))

    def publish_user(self, path, data, mode=0o644):
        self.user('python3', '-I', '-c', program(), str(path), format(mode, '04o'), input=data)

    def psql(self, database, sql):
        return self.user('podman', 'exec', '-i', 'trachuan-postgres',
                         '/usr/local/bin/postgres-admin.sh', 'psql', '-XAt',
                         '-h', '127.0.0.1', '-U', 'postgres', '-d', database,
                         '-v', 'ON_ERROR_STOP=1', input=sql.encode())

    def schema(self):
        raw = self.psql('auth', 'SELECT coalesce(max(version_id),0) FROM goose_db_version;').decode().strip()
        if not re.fullmatch('[0-9]+', raw):
            raise ValueError('invalid auth schema version')
        return int(raw)

    def postgres_changed(self, candidate):
        wanted = candidate.files['quadlet/trachuan-postgres.container'].replace(
            b'@POSTGRES_IMAGE@', candidate.manifest['images']['postgres'].encode()).rstrip(b'\n') + b'\n'
        try:
            return self.paths.postgres_unit.read_bytes() != wanted
        except FileNotFoundError:
            return True

    def install(self, directory):
        self.runner(['bash', str(directory / 'install.sh'), str(directory)], timeout=900)

    def configure(self, candidate):
        atomic(self.paths.host / 'images.env', pins(candidate.manifest), 0o644)
        for name in ('server.env', 'web.env'):
            atomic(self.paths.host / name, candidate.files[name], 0o644)
        self.user('bash', str(self.paths.host / 'render-units.sh'),
                  str(self.paths.host / 'images.env'), str(self.paths.quadlet), str(self.paths.user_units))
        self.user('systemctl', '--user', 'daemon-reload')

    def restart_postgres(self):
        self.systemctl('restart', 'postgres')
        self.user('podman', 'exec', 'trachuan-postgres', '/usr/local/bin/postgres-admin.sh',
                  '/usr/local/bin/wait-postgres.sh', timeout=150)

    def restore_postgres(self, candidate):
        template = candidate.files['quadlet/trachuan-postgres.container']
        for line in template.decode().splitlines():
            if not line.startswith('Volume='):
                continue
            source = line.split('=', 1)[1].split(':', 1)[0]
            if source.startswith('/srv/trachuan/host/'):
                name = source[len('/srv/trachuan/host/'):]
            elif source == '/etc/pgbackrest/pgbackrest.conf':
                name = 'pgbackrest.conf'
            else:
                continue
            atomic(self.paths.root / source.lstrip('/'), candidate.files[name],
                   0o755 if name.endswith('.sh') else 0o644)
        image = candidate.manifest['images']['postgres']
        self.publish_user(self.paths.postgres_unit, template.replace(b'@POSTGRES_IMAGE@', image.encode()))
        self.user('systemctl', '--user', 'daemon-reload')
        pull_image(image, self.runner, self.user_prefix)
        self.restart_postgres()

    def postgres(self):
        self.restart_postgres()
        self.user('podman', 'exec', 'trachuan-postgres', 'pgbackrest', '--stanza=trachuan', 'check')
        database = self.psql('auth', "SELECT active_db FROM corpus_release WHERE country = 'vn';").decode().strip()
        if not re.fullmatch(r'corpus_vn_[0-9]{8}_[0-9]+', database):
            raise ValueError('invalid active corpus database')
        self.psql(database, "BEGIN; CREATE EXTENSION pg_prewarm; "
                  "SELECT sum(pg_prewarm(c.oid)) FROM pg_class c "
                  "JOIN pg_namespace n ON n.oid = c.relnamespace "
                  "WHERE n.nspname IN ('public', 'pg_toast') AND c.relkind IN ('r', 'i', 't'); "
                  "DROP EXTENSION pg_prewarm; COMMIT;")

    def provision_activator(self):
        available = True
        for path in ('/usr/local/bin/provision-activator.sh', '/run/secrets/activator_db_password'):
            try:
                self.user('podman', 'exec', 'trachuan-postgres', 'test', '-r', path)
            except CommandError as error:
                if error.returncode != 1:
                    raise
                available = False
        if not available:
            return False
        self.user('podman', 'exec', 'trachuan-postgres', '/usr/local/bin/postgres-admin.sh',
                  'bash', '/usr/local/bin/provision-activator.sh')
        return True

    def migrate(self):
        self.provision_activator()
        self.systemctl('start', 'queryembed')
        self.systemctl('start', 'migrate')
        result = self.user('systemctl', '--user', 'show', '--property=Result', '--value', 'trachuan-migrate.service')
        if result.strip() != b'success':
            raise ValueError('auth migration failed')

    def probe(self, edge=False):
        args = ['curl', '--silent', '--show-error', '--fail', '--output', '/dev/null',
                '--write-out', '%{http_code}', '--retry', '30', '--retry-all-errors',
                '--retry-delay', '1', '--retry-max-time', '90', '--max-time', '3']
        if edge:
            args += ['--resolve', 'trachuan.vn:443:127.0.0.1', 'https://trachuan.vn/']
        else:
            args += ['http://127.0.0.1:8080/readyz']
        if self.runner(args, timeout=120).strip() != b'200':
            raise ValueError('edge probe failed' if edge else 'readiness failed')

    def confirm(self, candidate):
        installed = {}
        for line in (self.paths.host / 'images.env').read_text().splitlines():
            if line and not line.startswith('#'):
                key, value = line.split('=', 1)
                if key in installed:
                    raise ValueError('duplicate installed image')
                installed[key] = value
        wanted = dict(line.split('=', 1) for line in pins(candidate.manifest).decode().splitlines())
        if installed != wanted or self.postgres_changed(candidate):
            raise ValueError('bootstrap images or PostgreSQL unit do not match manifest')
        for name in ('server.env', 'web.env'):
            if (self.paths.host / name).read_bytes() != candidate.files[name]:
                raise ValueError('bootstrap env does not match bundle')
        images = dict(candidate.manifest['images'], **candidate.manifest['upstream'])
        for name, reference in images.items():
            actual = self.user('podman', 'container', 'inspect', '--format', '{{.ImageName}}', 'trachuan-' + name)
            if actual.decode().strip() != reference:
                raise ValueError('bootstrap running image mismatch')
            running = self.user('podman', 'container', 'inspect', '--format', '{{.State.Running}}', 'trachuan-' + name)
            if running.strip() != b'true':
                raise ValueError('bootstrap container not running')
        self.probe()
        self.probe(edge=True)
