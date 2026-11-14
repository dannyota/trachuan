import hashlib
from pathlib import PurePosixPath
import tarfile


def bundle(path, manifest):
    metadata = manifest['host_bundle']
    if path.stat().st_size != metadata['size']:
        raise ValueError('bundle size mismatch')
    if hashlib.sha256(path.read_bytes()).hexdigest() != metadata['sha256']:
        raise ValueError('bundle hash mismatch')
    try:
        return contents(path)
    except (tarfile.TarError, EOFError, OSError) as error:
        raise ValueError('invalid bundle archive') from error


def contents(path):
    files, seen, total = {}, set(), 0
    with tarfile.open(path, 'r:gz') as archive:
        for member in archive:
            parts = PurePosixPath(member.name).parts
            normalized = '/'.join(parts)
            if member.name.startswith('/') or '..' in parts or normalized in seen:
                raise ValueError('unsafe bundle path')
            seen.add(normalized)
            if len(seen) > 8192 or not (member.isdir() or member.isfile()):
                raise ValueError('unsafe bundle member')
            if parts in (('deploy',), ('deploy', 'host')) and member.isdir():
                continue
            if parts[:2] != ('deploy', 'host') or len(parts) < 3:
                raise ValueError('unsafe bundle root')
            if member.isdir():
                continue
            total += member.size
            if member.size < 0 or total > 64 * 1024 * 1024:
                raise ValueError('bundle expanded size limit')
            name = '/'.join(parts[2:])
            files[name] = archive.extractfile(member).read()
    if not {'install.sh', 'server.env', 'web.env', 'quadlet/trachuan-postgres.container'} <= files.keys():
        raise ValueError('incomplete host bundle')
    for name in ('server.env', 'web.env'):
        if b'REPLACE' in files[name]:
            raise ValueError('placeholder env')
    return files
