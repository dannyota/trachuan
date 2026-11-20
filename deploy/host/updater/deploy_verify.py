import base64
import hashlib
import json
from pathlib import Path
import pwd
import re
import subprocess

from .deploy_vault import names
from .deploy_floor import release_floor
from .deploy_bundle import bundle
from .deploy_io import canonical, command, CommandError


COSIGN = '/usr/local/libexec/trachuan/bin/cosign'
IDENTITIES = ('release_identity', 'build_identity', 'rollback_identity', 'corpus_identity')
NAMES = ('server', 'web', 'queryembed', 'postgres')


def match(pattern, value, field):
    if not isinstance(value, str) or re.fullmatch(pattern, value) is None:
        raise ValueError('invalid ' + field)


def positive(value, field):
    if type(value) is not int or value <= 0:
        raise ValueError('invalid ' + field)


def anchored(expression, field):
    if not isinstance(expression, str) or not expression.startswith('^') or not expression.endswith('$'):
        raise ValueError(field + ' must be anchored')
    try:
        re.compile(expression)
    except re.error as error:
        raise ValueError('invalid ' + field) from error


def keyed(value, repositories, field):
    if not isinstance(value, dict) or set(value) != set(repositories):
        raise ValueError(field + ' must be keyed by repository')
    return value.items()


def repositories(value):
    if (not isinstance(value, list) or not 1 <= len(value) <= 2
            or not all(isinstance(name, str) for name in value) or len(set(value)) != len(value)):
        raise ValueError('invalid policy repositories')
    for name in value:
        match(r'[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+', name, 'repository')
    return value


def accepted(value, rules, field):
    if not isinstance(value, str) or value not in rules['repositories']:
        raise ValueError('invalid ' + field)
    return value


def frameworks_policy(value):
    block = value['frameworks']
    if not isinstance(block, dict) or set(block) != {'repository', 'identity', 'namespace', 'bucket', 'min_release_floor'}:
        raise ValueError('invalid frameworks policy fields')
    match(r'ghcr\.io/[a-z0-9_./-]+', block['repository'], 'frameworks repository')
    for repository, identity in keyed(block['identity'], value['repositories'], 'frameworks identity'):
        anchored(identity, 'frameworks identity')
        if identity == value['corpus_identity'][repository]:
            raise ValueError('frameworks identity must differ from corpus identity')
    match(r'[a-z0-9]+', block['namespace'], 'frameworks namespace')
    match(r'[a-z0-9][a-z0-9-]{0,62}', block['bucket'], 'frameworks bucket')
    positive(block['min_release_floor'], 'frameworks release floor')


def policy(path):
    value = json.loads(Path(path).read_bytes())
    keys = {'version', 'repositories', 'release_repository', 'rollback_repository',
            'release_identity', 'build_identity', 'rollback_identity', 'issuer',
            'workflow_path', 'images', 'upstream', 'extensions', 'sigstore_trust_root',
            'vault_id', 'region', 'secrets', 'corpus_repository', 'corpus_identity',
            'release_namespace', 'release_bucket'}
    if not isinstance(value, dict) or set(value) - {'release_floor', 'frameworks'} != keys or value['version'] != 1:
        raise ValueError('invalid policy fields')
    release_floor(value)
    match(r'ocid1\.vault\.[A-Za-z0-9_.-]+', value['vault_id'], 'vault id')
    match(r'[a-z]+-[a-z]+-[0-9]+', value['region'], 'region')
    secrets = value['secrets']
    if (not isinstance(secrets, list) or not all(isinstance(name, str) for name in secrets)
            or sorted(secrets) != sorted(names())):
        raise ValueError('invalid policy secret names')
    if value['sigstore_trust_root'] != 'cosign-v3.1.4':
        raise ValueError('unsupported trust root')
    listed = repositories(value['repositories'])
    for key in IDENTITIES:
        for _, expression in keyed(value[key], listed, key):
            anchored(expression, 'policy identity')
    for repository, images in keyed(value['images'], listed, 'policy images'):
        if not isinstance(images, dict) or set(images) != set(NAMES):
            raise ValueError('invalid policy images')
        for image in images.values():
            match(r'ghcr\.io/' + re.escape(repository.split('/')[0]) + r'/[a-z0-9_./-]+', image, 'policy image')
    if not isinstance(value['upstream'], dict) or set(value['upstream']) != {'caddy'}:
        raise ValueError('invalid policy images')
    for repository in value['upstream'].values():
        match(r'[a-z0-9.-]+/[a-z0-9_./-]+', repository, 'policy image')
    for name in ('release_repository', 'rollback_repository', 'corpus_repository'):
        match(r'ghcr\.io/[a-z0-9_./-]+', value[name], name)
    match(r'[a-z0-9]+', value['release_namespace'], 'release namespace')
    match(r'[a-z0-9][a-z0-9-]{0,62}', value['release_bucket'], 'release bucket')
    if not isinstance(value['extensions'], list) or value['extensions'] not in (['vector','ltree'], ['ltree','vector']):
        raise ValueError('invalid extension allowlist')
    if 'frameworks' in value:
        frameworks_policy(value)
    return value


def release(data, rules):
    obj = json.loads(data)
    keys = {'kind', 'version', 'repository', 'commit', 'ref', 'release', 'sequence',
            'target', 'images', 'upstream', 'host_bundle', 'corpus_release', 'rollback_target'}
    if not isinstance(obj, dict) or set(obj) != keys:
        raise ValueError('invalid release fields')
    if obj['corpus_release'] is not None:
        raise ValueError('paired corpus and image activation is not implemented')
    repository = accepted(obj['repository'], rules, 'repository')
    for key, value in {'kind': 'trachuan.image-release', 'version': 1, 'target': 'production',
                       'corpus_release': None, 'rollback_target': None}.items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError('invalid ' + key)
    match(r'[0-9a-f]{40}', obj['commit'], 'commit')
    match(r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+', obj['ref'], 'release ref')
    if obj['release'] != obj['ref'][10:]:
        raise ValueError('release does not match ref')
    positive(obj['sequence'], 'sequence')
    for kind, names in (('images', rules['images'][repository]), ('upstream', rules['upstream'])):
        if not isinstance(obj[kind], dict) or set(obj[kind]) != set(names):
            raise ValueError('unlisted or missing image')
        for name, image in names.items():
            match(re.escape(image) + r'@sha256:[0-9a-f]{64}', obj[kind][name], 'image digest')
    metadata = obj['host_bundle']
    if not isinstance(metadata, dict) or set(metadata) != {'sha256', 'size'}:
        raise ValueError('invalid host bundle fields')
    match(r'[0-9a-f]{64}', metadata['sha256'], 'bundle hash')
    positive(metadata['size'], 'bundle size')
    if canonical(obj) != data:
        raise ValueError('manifest is not canonical JSON')
    return obj


def rollback(data):
    obj = json.loads(data)
    if not isinstance(obj, dict) or set(obj) != {'kind', 'version', 'current', 'target', 'sequence'}:
        raise ValueError('invalid rollback fields')
    if obj['kind'] != 'trachuan.rollback' or type(obj['version']) is not int or obj['version'] != 1:
        raise ValueError('invalid rollback kind or version')
    for field in ('current', 'target'):
        match('[0-9a-f]{64}', obj[field], field)
    if obj['current'] == obj['target']:
        raise ValueError('rollback target equals current')
    positive(obj['sequence'], 'sequence')
    if canonical(obj) != data:
        raise ValueError('manifest is not canonical JSON')
    return obj


def flags(rules, kind, repository, ref, commit=None):
    args = ['--certificate-identity-regexp', rules[kind + '_identity'][repository],
            '--certificate-oidc-issuer', rules['issuer'],
            '--certificate-github-workflow-repository', repository,
            '--certificate-github-workflow-ref', ref]
    if commit is not None:
        args += ['--certificate-github-workflow-sha', commit]
    return args


def verify_blob(directory, rules, runner, kind, repository, ref, commit=None):
    runner([COSIGN, 'verify-blob', '--bundle', str(Path(directory) / 'manifest.sigstore.json'),
            *flags(rules, kind, repository, ref, commit), str(Path(directory) / 'manifest.json')])


def verify_manifest(directory, rules, runner=command, kind='release'):
    data = (Path(directory) / 'manifest.json').read_bytes()
    if kind == 'release':
        obj = release(data, rules)
        verify_blob(directory, rules, runner, kind, obj['repository'], obj['ref'], obj['commit'])
        return hashlib.sha256(data).hexdigest(), obj
    obj = rollback(data)
    for index, repository in enumerate(rules['repositories']):
        try:
            verify_blob(directory, rules, runner, kind, repository, 'refs/heads/main')
            return hashlib.sha256(data).hexdigest(), obj
        except CommandError as error:
            if index == len(rules['repositories']) - 1 or error.returncode is None or error.returncode < 0:
                raise


class SubjectMismatch(ValueError):
    pass


def provenance(obj, manifest, image, rules):
    try:
        if obj['_type'] != 'https://in-toto.io/Statement/v1' or obj['predicateType'] != 'https://slsa.dev/provenance/v1':
            raise ValueError('invalid attestation type')
        repository, digest = image.split('@sha256:')
        if not any(subject['digest']['sha256'] == digest and subject['name'] == repository
                   for subject in obj['subject']):
            raise SubjectMismatch('wrong attestation subject digest')
        definition = obj['predicate']['buildDefinition']
        workflow = definition['externalParameters']['workflow']
        source = 'https://github.com/' + manifest['repository']
        if workflow['ref'] != manifest['ref'] or workflow['repository'] != source or workflow['path'] != rules['workflow_path']:
            raise ValueError('wrong attestation workflow')
        dependency = definition['resolvedDependencies'][0]
        if dependency['uri'] != 'git+' + source + '@' + manifest['ref'] or dependency['digest']['gitCommit'] != manifest['commit']:
            raise ValueError('wrong attestation source commit')
    except (KeyError, IndexError, TypeError) as error:
        raise ValueError('missing attestation claim') from error


def envelopes(raw):
    try:
        decoder, text, rows = json.JSONDecoder(), raw.decode(), []
        while text.strip():
            item, end = decoder.raw_decode(text.lstrip())
            rows.extend(item if isinstance(item, list) else [item])
            text = text.lstrip()[end:]
        if not rows:
            raise ValueError('missing attestation')
        return [json.loads(base64.b64decode(row['payload'], validate=True)) for row in rows]
    except (KeyError, TypeError, UnicodeError) as error:
        raise ValueError('invalid attestation envelope') from error


def as_user():
    uid = pwd.getpwnam('trachuan').pw_uid
    return ['runuser', '-u', 'trachuan', '--', 'env', 'XDG_RUNTIME_DIR=/run/user/' + str(uid),
            'DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/' + str(uid) + '/bus']


def pull_image(image, runner, user_prefix):
    inspect = user_prefix + ['podman', 'image', 'inspect', image]
    try:
        raw = runner(inspect)
    except CommandError:
        try:
            runner(user_prefix + ['podman', 'image', 'exists', image])
        except CommandError as error:
            if error.returncode != 1:
                raise
        else:
            raise
        pull = user_prefix + ['podman', 'pull', image]
        try:
            runner(pull, timeout=600)
        except CommandError as error:
            if not isinstance(error.__cause__, subprocess.TimeoutExpired):
                raise
            runner(pull, timeout=600)
        raw = runner(inspect)
    rows = json.loads(raw)
    if len(rows) != 1 or image not in rows[0].get('RepoDigests', []):
        raise ValueError('local digest mismatch')


class Verifier:
    def __init__(self, rules, runner=command, user_prefix=None):
        self.rules, self.runner = rules, runner
        self.user_prefix = as_user() if user_prefix is None else user_prefix

    def attestation(self, manifest, image, expected_image=None):
        raw = self.runner([COSIGN, 'verify-attestation', '--new-bundle-format', '--type', 'slsaprovenance1',
                           *flags(self.rules, 'build', manifest['repository'], manifest['ref'], manifest['commit']), image])
        statements = envelopes(raw)
        for statement in statements:
            provenance(statement, manifest, expected_image or image, self.rules)
        return statements

    def images(self, manifest):
        for kind in ('images', 'upstream'):
            for image in manifest[kind].values():
                pull_image(image, self.runner, self.user_prefix)
                if kind == 'upstream':
                    continue
                self.attestation(manifest, image)
