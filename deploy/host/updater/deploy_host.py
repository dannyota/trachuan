import re

from . import deploy_corpus_sql, deploy_frameworks_sql
from .deploy_corpus_sql import (AUTH_ACTIVE_DATABASE, AUTH_SCHEMA_VERSION, CATALOG_PATH, PREWARM, PREWARM_SKIPPED,
                                SCHEMA_PATH, database_owner)
from .deploy_io import atomic, command, CommandError, remove, Result
from .deploy_transfer import program
from .deploy_verify import as_user, pull_image


AUTH_OUTPUT_LIMIT = 4096
AUTH_CLIENT_SECONDS = 30
AUTH_OPTIONS = (SCHEMA_PATH + ' -c statement_timeout=10s -c lock_timeout=5s '
                '-c idle_in_transaction_session_timeout=10s -c client_connection_check_interval=2s')
RELEASE_OUTPUT_LIMIT = 4 * 1024 * 1024
RELEASE_CLIENT_SECONDS = 60
SCRAM_ONLY = ('--env', 'PGREQUIREAUTH=scram-sha-256')
CLIENT_SHARED = ('auth', 'postgres')
SUPERUSER_SHARED = ('postgres',)


def client_database(name, shared=CLIENT_SHARED):
    if name in shared:
        return name
    for check in (deploy_corpus_sql.database, deploy_frameworks_sql.database):
        try:
            check(name)
            return name
        except ValueError:
            pass
    raise ValueError('invalid database name')


def pins(manifest):
    images = dict(manifest['images'], **manifest['upstream'])
    return ''.join(name.upper() + '_IMAGE=' + image + '\n' for name, image in sorted(images.items())).encode()


def secret_arguments(secret, *args):
    return ['podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c',
            'set +x; set -eu; export PGPASSWORD PGREQUIREAUTH; PGREQUIREAUTH=scram-sha-256; '
            'PGPASSWORD=$(cat /run/secrets/' + secret + '); '
            '[ -n "$PGPASSWORD" ]; exec "$@"', 'sh', *args]


def activator_arguments(*args):
    return secret_arguments('activator_db_password', *args)


def activator_client(database, options=SCHEMA_PATH):
    return ('env', 'PGOPTIONS=' + options, 'psql', '--no-password', '-XqAt', '-h', '127.0.0.1',
            '-U', 'activator', '-v', 'ON_ERROR_STOP=1', '-d', client_database(database))


def auth_arguments(sql):
    return secret_arguments('auth_db_password', 'timeout', '--signal=TERM', '--kill-after=2s', '15s',
                            'env', 'PGCONNECT_TIMEOUT=5', 'PGOPTIONS=' + AUTH_OPTIONS, 'psql', '--no-password', '-XqAt',
                            '-h', '127.0.0.1', '-U', 'auth_owner', '-v', 'ON_ERROR_STOP=1', '-d', 'auth', '-c', sql)


def release_options(write):
    return (SCHEMA_PATH + ' -c statement_timeout=30s -c lock_timeout=5s -c idle_in_transaction_session_timeout=30s '
            '-c default_transaction_read_only=' + ('off' if write else 'on') +
            ' -c standard_conforming_strings=on -c client_encoding=UTF8 -c client_connection_check_interval=2s')


def release_arguments(write):
    return secret_arguments('auth_db_password', 'timeout', '--signal=TERM', '--kill-after=2s', '45s',
                            'env', 'PGCONNECT_TIMEOUT=5', 'PGOPTIONS=' + release_options(write), 'psql',
                            '--no-password', '-XqAt', '-h', '127.0.0.1', '-U', 'auth_owner',
                            '-v', 'ON_ERROR_STOP=1', '-d', 'auth')


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

    def note(self, step, message):
        try:
            self.warning(step, message)
        except OSError:
            pass

    def publish_user(self, path, data, mode=0o644):
        self.user('python3', '-I', '-c', program(), str(path), format(mode, '04o'), input=data)

    def psql(self, database, sql, options=CATALOG_PATH):
        return self.user('podman', 'exec', '-i', '--env', 'PGOPTIONS=' + options, *SCRAM_ONLY, 'trachuan-postgres',
                         '/usr/local/bin/postgres-admin.sh', 'psql', '-XqAt',
                         '-h', '127.0.0.1', '-U', 'postgres', '-d', client_database(database, SUPERUSER_SHARED),
                         '-v', 'ON_ERROR_STOP=1', input=sql.encode())

    def activator(self, *args, **kwargs):
        return self.user(*activator_arguments(*args), **kwargs)

    def activator_psql(self, database, sql, options=SCHEMA_PATH):
        return self.activator(*activator_client(database, options),
                              input=('SET standard_conforming_strings=on;\n' + sql).encode()).decode().strip()

    def auth_psql(self, sql):
        return self.user(*auth_arguments(sql), stdout_limit=AUTH_OUTPUT_LIMIT,
                         timeout=AUTH_CLIENT_SECONDS).decode().strip()

    def release_sql(self, sql, write=False):
        return self.user(*release_arguments(write), input=sql.encode(), stdout_limit=RELEASE_OUTPUT_LIMIT,
                         timeout=RELEASE_CLIENT_SECONDS).decode().strip()

    def schema(self):
        raw = self.auth_psql(AUTH_SCHEMA_VERSION)
        if not re.fullmatch('[0-9]{1,19}', raw):
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
        self.user('podman', 'exec', *SCRAM_ONLY, 'trachuan-postgres', '/usr/local/bin/postgres-admin.sh',
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
        self.user('podman', 'exec', *SCRAM_ONLY, 'trachuan-postgres', 'pgbackrest', '--stanza=trachuan', 'check')
        database = self.auth_psql(AUTH_ACTIVE_DATABASE)
        deploy_corpus_sql.database(database)
        self.prewarm_active(database)

    def prewarm_active(self, database):
        if not self.provision_activator():
            return self.prewarm_skipped('activator not provisioned')
        if self.psql('postgres', database_owner(database)).decode().strip() != 'activator':
            return self.prewarm_skipped('database not owned by activator')
        if self.activator_psql(database, PREWARM, CATALOG_PATH) == PREWARM_SKIPPED:
            self.prewarm_skipped('database has no pg_prewarm')

    def prewarm_skipped(self, reason):
        self.note('prewarm', 'active corpus prewarm skipped: ' + reason)

    def provision_activator(self):
        try:
            script = (self.paths.host / 'provision-activator.sh').read_bytes()
        except FileNotFoundError:
            return False
        available = True
        for path in ('/usr/local/bin/postgres-admin.sh', '/run/secrets/activator_db_password'):
            try:
                self.user('podman', 'exec', 'trachuan-postgres', 'test', '-r', path)
            except CommandError as error:
                if error.returncode != 1:
                    raise
                available = False
        if not available:
            return False
        self.user('podman', 'exec', '-i', *SCRAM_ONLY, 'trachuan-postgres', '/usr/local/bin/postgres-admin.sh',
                  'bash', '-s', '--', '--no-auth-tables', input=script)
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
