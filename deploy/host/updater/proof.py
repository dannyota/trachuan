import copy
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from updater.deploy_io import canonical, command, CommandError, Lock, Result
from updater.deploy_state import State
from updater.deploy_store import Paths, Store
from updater.deploy_verify import policy, verify_manifest


def snapshot(paths):
    result = {}
    for name in ('images.env', 'server.env', 'web.env'):
        path = paths.host / name
        if path.is_file():
            result[str(path)] = hashlib.sha256(path.read_bytes()).hexdigest()
    for directory in (paths.state, paths.quadlet, paths.user_units, paths.bundles):
        if directory.exists():
            for path in directory.rglob('*'):
                if path.is_file() and not path.is_symlink():
                    result[str(path)] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def rejected(callback):
    try:
        callback()
    except (ValueError, CommandError):
        return
    raise RuntimeError('proof candidate was accepted')


def run(paths, runner=command):
    if paths.mode.read_text().strip() != 'verify':
        raise ValueError('proof requires verify mode')
    rules = policy(paths.policy)
    with Lock(paths.lock), Lock(paths.backup_lock, create=False):
        before = snapshot(paths)
        state = State(paths.state / 'state.json')
        candidate = Store(paths, rules, runner).load(state.data['active'])
        logger = Result(paths.result, paths.log)
        names = []
        with tempfile.TemporaryDirectory(prefix='trachuan-proof-') as temporary:
            directory = Path(temporary)
            (directory / 'manifest.sigstore.json').write_bytes((candidate.directory / 'manifest.sigstore.json').read_bytes())
            dispatch = dict(candidate.manifest, ref='refs/heads/main')
            (directory / 'manifest.json').write_bytes(canonical(dispatch))
            rejected(lambda: verify_manifest(directory, rules, runner))
            names.append('dispatch-build')
            wrong = copy.deepcopy(candidate.manifest)
            image = wrong['images']['server']
            wrong['images']['server'] = image.split('@')[0] + '@sha256:' + ('0' if image[-1] != '0' else '1') * 64
            (directory / 'manifest.json').write_bytes(canonical(wrong))
            rejected(lambda: verify_manifest(directory, rules, runner))
            names.append('wrong-digest')
            replay = copy.deepcopy(state.data)
            auth = dict(kind='trachuan.rollback', version=1, current=state.data['active'],
                        target='0' * 64, sequence=1)
            auth_hash = hashlib.sha256(canonical(auth)).hexdigest()
            replay['consumed'].append(auth_hash)
            cloned = State(directory / 'replay.json', replay)
            rejected(lambda: cloned.authorize(auth_hash, auth, replay['schemas'][replay['active']]))
            names.append('replayed-rollback')
            older = copy.deepcopy(state.data)
            newer_hash = hashlib.sha256(b'trachuan proof sequence floor').hexdigest()
            older.update(active=newer_hash, retained=[], failed=[], journal=None)
            older['schemas'][newer_hash] = older['schemas'][state.data['active']]
            older['accepted']['image'] = {str(candidate.manifest['sequence'] + 1): {'hash': newer_hash, 'status': 'active'}}
            floor = State(directory / 'older.json', older)
            rejected(lambda: floor.accept('image', candidate.hash, candidate.manifest['sequence']))
            names.append('older-sequence')
        if snapshot(paths) != before:
            raise RuntimeError('proof changed host or deployment state')
        for name in names:
            logger.append(dict(status='rejected', proof=name, message='proof rejected; host and deployment state unchanged'))
        rows = [json.loads(line) for line in paths.log.read_text().splitlines()[-4:]]
        if [row.get('proof') for row in rows] != names or not all(row['status'] == 'rejected' for row in rows):
            raise RuntimeError('proof rejection missing from deploy.log')
        return 0


if __name__ == '__main__':
    if os.geteuid() != 0:
        sys.exit('deploy proof: run as root')
    try:
        sys.exit(run(Paths()))
    except Exception:
        sys.exit('deploy proof: failed; keep verify mode')
