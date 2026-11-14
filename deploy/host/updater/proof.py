import argparse
import copy
import hashlib
import json
import os
import re
from pathlib import Path
import sys
import tempfile

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from updater.deploy_io import canonical, command, CommandError, Lock, Result
from updater.deploy_state import State
from updater.deploy_store import Paths, Store
from updater.deploy_verify import envelopes, flags, match, policy, provenance, release, SubjectMismatch, Verifier


def snapshot(paths):
    result = {}
    files = [paths.host / name for name in ('images.env', 'server.env', 'web.env')]
    for path in [*files, paths.hold]:
        if path.is_file():
            result[str(path)] = hashlib.sha256(path.read_bytes()).hexdigest()
    for directory in (paths.state, paths.quadlet, paths.user_units, paths.bundles):
        if directory.exists():
            for path in directory.rglob('*'):
                if path.is_file() and not path.is_symlink():
                    result[str(path)] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def rejected(callback, expected):
    try:
        callback()
    except CommandError as error:
        if error.reason != expected or expected != 'identity':
            raise
    except SubjectMismatch:
        if expected != 'subject':
            raise
    except ValueError as error:
        stages = {'rollback consumed': 'rollback-consumption', 'older sequence': 'sequence-floor'}
        if stages.get(str(error)) != expected:
            raise
    else:
        raise RuntimeError('proof candidate was accepted')
    return expected


def dispatch_manifest(image, rules, runner):
    repositories = '|'.join(re.escape(name) for name in rules['images'].values())
    match('(?:' + repositories + ')@sha256:[0-9a-f]{64}', image, 'dispatch image')
    identity = '^' + re.escape('https://github.com/' + rules['repository'] + '/' + rules['workflow_path']) + '@refs/heads/main$'
    main_rules = dict(rules, build_identity=identity)
    raw = runner(['/usr/local/libexec/trachuan/bin/cosign', 'verify-attestation',
        '--new-bundle-format', '--type', 'slsaprovenance1',
        *flags(main_rules, 'build', 'refs/heads/main'),
        '--certificate-github-workflow-trigger', 'workflow_dispatch', image])
    statements = envelopes(raw)
    commit = statements[0]['predicate']['buildDefinition']['resolvedDependencies'][0]['digest']['gitCommit']
    match('[0-9a-f]{40}', commit, 'dispatch commit')
    manifest = {'ref': 'refs/heads/main', 'commit': commit}
    for statement in statements:
        provenance(statement, manifest, image, main_rules)
    return manifest


def run(paths, dispatch_image, runner=command):
    if paths.mode.read_text().strip() != 'verify':
        raise ValueError('proof requires verify mode')
    rules = policy(paths.policy)
    with Lock(paths.lock), Lock(paths.backup_lock, create=False):
        before = snapshot(paths)
        state = State(paths.state / 'state.json')
        candidate = Store(paths, rules, runner).load(state.data['active'])
        logger = Result(paths.result, paths.log)
        verifier = Verifier(rules, runner, user_prefix=[])
        main_manifest = dispatch_manifest(dispatch_image, rules, runner)
        stages = []
        stages.append(rejected(lambda: verifier.attestation(main_manifest, dispatch_image), 'identity'))
        recorded = release(Path(__file__).with_name('fixtures').joinpath('manifest.json').read_bytes(), rules)
        server = recorded['images']['server']
        wrong = server.split('@')[0] + '@' + recorded['images']['web'].split('@')[1]
        stages.append(rejected(lambda: verifier.attestation(recorded, server, expected_image=wrong), 'subject'))
        names = ['dispatch-build', 'wrong-digest', 'replayed-rollback', 'older-sequence']
        with tempfile.TemporaryDirectory(prefix='trachuan-proof-') as temporary:
            directory = Path(temporary)
            replay = copy.deepcopy(state.data)
            auth = dict(kind='trachuan.rollback', version=1, current=state.data['active'],
                        target='0' * 64, sequence=1)
            auth_hash = hashlib.sha256(canonical(auth)).hexdigest()
            replay['consumed'].append(auth_hash)
            cloned = State(directory / 'replay.json', replay)
            stages.append(rejected(lambda: cloned.check_rollback(auth_hash, auth,
                replay['schemas'][replay['active']]), 'rollback-consumption'))
            older = copy.deepcopy(state.data)
            newer_hash = hashlib.sha256(b'trachuan proof sequence floor').hexdigest()
            older.update(active=newer_hash, retained=[], failed=[], journal=None)
            older['schemas'][newer_hash] = older['schemas'][state.data['active']]
            older['accepted']['image'] = {str(candidate.manifest['sequence'] + 1): {'hash': newer_hash, 'status': 'active'}}
            floor = State(directory / 'older.json', older)
            stages.append(rejected(lambda: floor.accept('image', candidate.hash,
                candidate.manifest['sequence']), 'sequence-floor'))
        if snapshot(paths) != before:
            raise RuntimeError('proof changed host or deployment state')
        for name, stage in zip(names, stages):
            logger.append(dict(status='rejected', proof=name, stage=stage,
                               message='proof rejected; host and deployment state unchanged'))
        rows = [json.loads(line) for line in paths.log.read_text().splitlines()[-4:]]
        if [row.get('proof') for row in rows] != names or not all(row['status'] == 'rejected' for row in rows):
            raise RuntimeError('proof rejection missing from deploy.log')
        if [row.get('stage') for row in rows] != stages:
            raise RuntimeError('proof rejection stage missing from deploy.log')
        return 0


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('dispatch_image')
    args = parser.parse_args()
    if os.geteuid() != 0:
        sys.exit('deploy proof: run as root')
    try:
        sys.exit(run(Paths(), args.dispatch_image))
    except Exception:
        sys.exit('deploy proof: failed; keep verify mode')
