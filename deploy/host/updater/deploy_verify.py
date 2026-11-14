import base64
import hashlib
import json
from pathlib import Path
import pwd
import re

from .deploy_bundle import bundle
from .deploy_io import canonical, command, CommandError


def match(pattern, value, field):
    if not isinstance(value, str) or re.fullmatch(pattern, value) is None:
        raise ValueError('invalid ' + field)


def positive(value, field):
    if type(value) is not int or value <= 0:
        raise ValueError('invalid ' + field)


def policy(path):
    value = json.loads(Path(path).read_bytes())
    keys = {'version', 'repository', 'release_repository', 'rollback_repository',
            'release_identity', 'build_identity', 'rollback_identity', 'issuer',
            'workflow_path', 'images', 'upstream', 'extensions', 'sigstore_trust_root'}
    if not isinstance(value, dict) or set(value) != keys or value['version'] != 1:
        raise ValueError('invalid policy fields')
    if value['sigstore_trust_root'] != 'cosign-v3.1.4':
        raise ValueError('unsupported trust root')
    for key in ('release_identity', 'build_identity', 'rollback_identity'):
        expression = value[key]
        if not isinstance(expression, str) or not expression.startswith('^') or not expression.endswith('$'):
            raise ValueError('policy identity must be anchored')
        re.compile(expression)
    if set(value['images']) != {'server', 'web', 'queryembed', 'postgres'} or set(value['upstream']) != {'caddy'}:
        raise ValueError('invalid policy images')
    for name in ('release_repository', 'rollback_repository'):
        match(r'ghcr\.io/[a-z0-9_./-]+', value[name], name)
    for name in ('images', 'upstream'):
        for repository in value[name].values():
            match(r'[a-z0-9.-]+/[a-z0-9_./-]+', repository, 'policy image')
    match(r'[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+', value['repository'], 'repository')
    return value


def release(data, rules):
    obj = json.loads(data)
    keys = {'kind', 'version', 'repository', 'commit', 'ref', 'release', 'sequence',
            'target', 'images', 'upstream', 'host_bundle', 'corpus_release', 'rollback_target'}
    if not isinstance(obj, dict) or set(obj) != keys:
        raise ValueError('invalid release fields')
    for key, value in {'kind': 'trachuan.image-release', 'version': 1,
                       'repository': rules['repository'], 'target': 'production',
                       'corpus_release': None, 'rollback_target': None}.items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError('invalid ' + key)
    match(r'[0-9a-f]{40}', obj['commit'], 'commit')
    match(r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+', obj['ref'], 'release ref')
    if obj['release'] != obj['ref'][10:]:
        raise ValueError('release does not match ref')
    positive(obj['sequence'], 'sequence')
    for kind in ('images', 'upstream'):
        if not isinstance(obj[kind], dict) or set(obj[kind]) != set(rules[kind]):
            raise ValueError('unlisted or missing image')
        for name, repository in rules[kind].items():
            match(re.escape(repository) + r'@sha256:[0-9a-f]{64}', obj[kind][name], 'image digest')
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


def flags(rules, kind, ref, commit=None):
    args = ['--certificate-identity-regexp', rules[kind + '_identity'],
            '--certificate-oidc-issuer', rules['issuer'],
            '--certificate-github-workflow-repository', rules['repository'],
            '--certificate-github-workflow-ref', ref]
    if commit is not None:
        args += ['--certificate-github-workflow-sha', commit]
    return args


def verify_manifest(directory, rules, runner=command, kind='release'):
    path = Path(directory) / 'manifest.json'
    data = path.read_bytes()
    obj = release(data, rules) if kind == 'release' else rollback(data)
    ref, commit = (obj['ref'], obj['commit']) if kind == 'release' else ('refs/heads/main', None)
    runner(['/usr/local/libexec/trachuan/bin/cosign', 'verify-blob', '--bundle',
            str(Path(directory) / 'manifest.sigstore.json'), *flags(rules, kind, ref, commit), str(path)])
    return hashlib.sha256(data).hexdigest(), obj


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
        source = 'https://github.com/' + rules['repository']
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
        runner(user_prefix + ['podman', 'pull', image], timeout=600)
        raw = runner(inspect)
    rows = json.loads(raw)
    if len(rows) != 1 or image not in rows[0].get('RepoDigests', []):
        raise ValueError('local digest mismatch')


class Verifier:
    def __init__(self, rules, runner=command, user_prefix=None):
        self.rules, self.runner = rules, runner
        self.user_prefix = as_user() if user_prefix is None else user_prefix

    def attestation(self, manifest, image, expected_image=None):
        raw = self.runner(['/usr/local/libexec/trachuan/bin/cosign', 'verify-attestation',
            '--new-bundle-format', '--type', 'slsaprovenance1',
            *flags(self.rules, 'build', manifest['ref'], manifest['commit']), image])
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
