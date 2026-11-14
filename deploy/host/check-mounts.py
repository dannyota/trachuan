from pathlib import Path
import shlex
import subprocess
import tempfile

HOST = Path(__file__).resolve().parent
PODMAN_IMAGE = 'quay.io/podman/stable@sha256:663e0dbf407987b7db3f20d3588c283a8228db17b282d2029a482d4d47e36964'
PROBE_IMAGE = 'docker.io/library/debian@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2'


def quadlet_mounts(units):
    mounts = []
    for unit in sorted(units.glob('*.container')):
        section = ''
        user = ''
        values = []
        for line in unit.read_text().splitlines():
            line = line.strip()
            if line.startswith('['):
                section = line
            if section != '[Container]':
                continue
            key, _, value = line.partition('=')
            if key == 'User':
                user = value
            elif key in ('Tmpfs', 'Mount'):
                values.append(('--tmpfs' if key == 'Tmpfs' else '--mount', value))
            elif key == 'Secret' and 'type=mount' in value.split(','):
                values.append(('--secret', value))
        mounts.extend((unit.name, user, flag, value) for flag, value in values)
    return mounts


def script_mounts(script):
    mounts = []
    for line in script.read_text().replace('\\\n', '').splitlines():
        args = shlex.split(line, comments=True)
        if args[:2] != ['podman', 'run']:
            continue
        user = ''
        values = []
        args = iter(args[2:])
        for arg in args:
            key, separator, value = arg.partition('=')
            if key not in ('--tmpfs', '--mount', '--user', '-u'):
                continue
            if not separator:
                value = next(args)
            if key in ('--user', '-u'):
                user = value
            else:
                values.append((key, value))
        mounts.extend((script.name, user, flag, value) for flag, value in values)
    return mounts


def required(mounts, source, destinations):
    found = set()
    for name, _, flag, value in mounts:
        if name != source:
            continue
        if flag == '--mount':
            found.update(part.split('=', 1)[1] for part in value.split(',') if part.startswith('destination='))
        else:
            found.add(value.split(':', 1)[0])
    missing = destinations - found
    if missing:
        raise ValueError(f'{source}: missing mounts {sorted(missing)}')
    return mounts


def probe_script(mounts):
    if not mounts:
        raise ValueError('no mounts found')
    podman = ['podman', '--storage-driver=vfs']
    commands = [shlex.join([*podman, 'pull', PROBE_IMAGE])]
    for name in sorted({value.split(',', 1)[0] for _, _, flag, value in mounts if flag == '--secret'}):
        commands.append(f"printf probe | {shlex.join([*podman, 'secret', 'create', name, '-'])}")
    for source, user, flag, value in mounts:
        if flag == '--secret':
            options = dict(option.partition('=')[::2] for option in value.split(',')[1:])
            path = options.get('target', '/run/secrets/' + value.split(',', 1)[0])
            if not path.startswith('/'):
                path = '/run/secrets/' + path
            expected = f"{options.get('uid', '0')}:{options.get('gid', '0')} {options.get('mode', '0444').lstrip('0')}"
            args = [*podman, 'run', '--rm', '--pull=never', '--cgroups=disabled', '--network=none']
            if user:
                args.extend(['--user', user])
            args.extend([flag, value, PROBE_IMAGE, 'sh', '-ec',
                         'stat -c "%u:%g %a" "$1"; test "$(stat -c "%u:%g %a" "$1")" = "$2"; test "$(cat "$1")" = probe',
                         'sh', path, expected])
            commands.append(shlex.join(['printf', '%s\\n', f'{source}: {flag} {value}']))
            commands.append(shlex.join(args))
            continue
        if flag == '--tmpfs':
            destination = value.split(':', 1)[0]
        else:
            options = dict(option.partition('=')[::2] for option in value.split(','))
            destination = next(options[key] for key in ('destination', 'dst', 'target') if key in options)
        args = [*podman, 'run', '--rm', '--pull=never', '--cgroups=disabled', '--network=none']
        if user:
            args.extend(['--user', user])
        args.extend([flag, value, PROBE_IMAGE])
        if destination in ('/run/pgbackrest', '/run/restore-check'):
            args.extend(['sh', '-ec',
                         'stat -c "%u:%g %a" "$1"; '
                         'test "$(stat -c "%u:%g %a" "$1")" = "999:999 700"; '
                         'touch "$1/.mount-check"', 'sh', destination])
        else:
            args.append('true')
        commands.append(shlex.join(['printf', '%s\\n', f'{source}: {flag} {value}']))
        commands.append(shlex.join(args))
    return '\n'.join(commands)


def main():
    with tempfile.TemporaryDirectory(prefix='trachuan-mounts-test-') as work:
        work = Path(work)
        pins = work / 'images.env'
        repositories = {
            'SERVER': 'ghcr.io/dannyota/trachuan-server',
            'WEB': 'ghcr.io/dannyota/trachuan-web',
            'QUERYEMBED': 'ghcr.io/dannyota/trachuan-queryembed',
            'CADDY': 'docker.io/library/caddy',
            'POSTGRES': 'ghcr.io/dannyota/trachuan-postgres',
        }
        pins.write_text(''.join(f'{name}_IMAGE={repo}@sha256:{"1" * 64}\n'
                                for name, repo in repositories.items()))
        units = work / 'units'
        subprocess.run(['bash', str(HOST / 'render-units.sh'), str(pins), str(units)], check=True)
        mounts = required(quadlet_mounts(units), 'trachuan-postgres.container', {'/run/pgbackrest'}) + required(
            script_mounts(HOST / 'restore-check.sh'), 'restore-check.sh', {'/run/pgbackrest', '/run/restore-check', '/tmp'})
        subprocess.run(['docker', 'run', '--rm', '--privileged', '--entrypoint=/bin/sh',
                        PODMAN_IMAGE, '-euc', probe_script(mounts)], check=True, timeout=170)


if __name__ == '__main__':
    main()
