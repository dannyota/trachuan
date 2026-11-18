from contextlib import contextmanager
import json
import os
from pathlib import Path
import shutil
import stat

SYNC_ROOTS = ('Library/Mobile Documents', 'Library/CloudStorage', 'Dropbox', 'Google Drive', 'Desktop', 'Documents')
PRIVATE = '.upload'


def real_path(path):
    path = os.fspath(path)
    try:
        current = os.sep if os.path.isabs(path) else os.path.realpath(os.getcwd(), strict=True)
        missing, directory = False, True
        for part in path.split(os.sep):
            if part and not directory:
                raise ValueError('resolve path: parent is not a directory')
            if part in ('', '.'):
                continue
            if part == '..':
                if missing:
                    raise ValueError('resolve path: missing parent')
                current = os.path.dirname(current)
                continue
            current = os.path.join(current, part)
            if missing:
                continue
            try:
                info = os.lstat(current)
            except FileNotFoundError:
                missing = True
                continue
            if stat.S_ISLNK(info.st_mode):
                current = os.path.realpath(current, strict=True)
                info = os.stat(current)
            directory = stat.S_ISDIR(info.st_mode)
    except OSError as error:
        raise ValueError('resolve path failed') from error
    return current


def folded(path):
    return ''.join(char.lower() for char in path)


def contained(path, root):
    return path == root or path.startswith(root.rstrip(os.sep) + os.sep)


def inside(path, root):
    return contained(folded(path), folded(root))


def refuse(root):
    raise ValueError('sync folder refused: ' + root)


def reported_roots(home):
    try:
        data = Path(home, '.dropbox/info.json').read_bytes()
    except FileNotFoundError:
        return []
    except OSError as error:
        raise ValueError('read sync configuration failed') from error
    try:
        clients = json.loads(data)
    except (ValueError, RecursionError) as error:
        raise ValueError('decode sync configuration failed') from error
    if not isinstance(clients, dict):
        raise ValueError('decode sync configuration failed')
    roots = []
    for client in clients.values():
        if client is None:
            continue
        if not isinstance(client, dict):
            raise ValueError('decode sync configuration failed')
        for key, value in client.items():
            if key.lower() != 'path':
                continue
            if not isinstance(value, str):
                raise ValueError('decode sync configuration failed')
            if value:
                roots.append(value)
    return roots


def check_sync(path, home):
    home = real_path(home)
    roots = list(SYNC_ROOTS)
    try:
        entries = os.listdir(home)
    except OSError as error:
        raise ValueError('inspect sync folders failed') from error
    roots += [entry for entry in entries if entry.lower().startswith('onedrive')]
    roots.append('OneDrive')
    if inside(path, home) and len(path) > len(home):
        first = path[len(home):].lstrip(os.sep).split(os.sep, 1)[0]
        if first.lower().startswith('onedrive'):
            roots.append(first)
    for root in roots:
        resolved = real_path(os.path.join(home, root))
        if inside(path, resolved):
            refuse(resolved)
    for root in reported_roots(home):
        resolved = real_path(root)
        if inside(path, resolved):
            refuse(resolved)
    ancestor = path
    while True:
        try:
            info = os.lstat(os.path.join(ancestor, '.dropbox'))
        except FileNotFoundError:
            info = None
        except OSError as error:
            raise ValueError('inspect sync marker failed') from error
        if info is not None and (not stat.S_ISDIR(info.st_mode) or ancestor != home):
            refuse(ancestor)
        parent = os.path.dirname(ancestor)
        if parent == ancestor:
            return
        ancestor = parent


def checked_directory(path):
    resolved = real_path(path)
    home = os.environ.get('HOME')
    if not home:
        raise ValueError('HOME is required')
    check_sync(resolved, home)
    return resolved


def release_directory(rel):
    value = os.environ.get('FRAMEWORKS_WORK_DIR')
    if not value:
        raise ValueError('FRAMEWORKS_WORK_DIR is required')
    work = checked_directory(value)
    base = checked_directory(os.path.join(work, rel))
    if not contained(base, work) or base == work:
        raise ValueError('release directory outside work directory')
    if not os.path.isdir(base):
        raise ValueError('frameworks release directory missing')
    return Path(base)


def source_file(base, relative):
    path = os.path.join(str(base), relative)
    parent = checked_directory(os.path.dirname(path))
    if not contained(parent, str(base)):
        raise ValueError('frameworks input outside release directory: ' + relative)
    candidate = os.path.join(parent, os.path.basename(path))
    try:
        info = os.lstat(candidate)
    except (FileNotFoundError, NotADirectoryError):
        raise ValueError('missing frameworks input: ' + relative) from None
    if not stat.S_ISREG(info.st_mode):
        raise ValueError('frameworks input is not a regular file: ' + relative)
    return Path(candidate)


def remove_private(path):
    if os.path.islink(path):
        raise ValueError('private directory is a symlink')
    if os.path.lexists(path):
        shutil.rmtree(path)


@contextmanager
def private_directory(base):
    path = os.path.join(str(base), PRIVATE)
    resolved = checked_directory(path)
    if resolved != path or not contained(resolved, str(base)):
        raise ValueError('private directory outside release directory')
    remove_private(path)
    os.mkdir(path, 0o700)
    try:
        os.chmod(path, 0o700)
        yield Path(path)
    finally:
        remove_private(path)
