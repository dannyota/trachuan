import copy
import hashlib
import json
from pathlib import Path

REL = '20990102'
PREVIOUS = '20990101'
SENTINEL = 'LICENSED-SENTINEL-TEXT'
COMMIT = 'a' * 40
OBJECTS = {'frameworks.dump': 'build/frameworks.dump', 'build.json': 'build/build.json',
           'evaluate.json': 'evaluate/manifest.json', 'report.json': 'evaluate/report.json',
           'release.json': 'release/release.json'}
STEPS = ('snapshot', 'parse', 'relate', 'chunk', 'embed')

FAKE = r'''#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sys
import time

args = sys.argv[1:]
name = Path(sys.argv[0]).name
remote = Path(os.environ.get('FAKE_REMOTE', '/nonexistent'))
registry = Path(os.environ.get('FAKE_REGISTRY', '/nonexistent'))
temporary = os.environ.get('TMPDIR', '')
mode = oct(os.stat(temporary).st_mode & 0o777) if temporary and os.path.isdir(temporary) else None
config = os.environ.get('AWS_CONFIG_FILE', '')


def option(key):
    return args[args.index(key) + 1]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def etag(data):
    return '"' + sha(data) + '"'


def table(variable):
    return json.loads(os.getenv(variable, '{}'))


def artifact(directory):
    kind = os.getenv('FAKE_ARTIFACT_TYPE', 'application/vnd.trachuan.corpus-release.v1')
    layers = [dict(digest='sha256:' + sha((directory / title).read_bytes()), size=(directory / title).stat().st_size,
                   annotations={'org.opencontainers.image.title': title})
              for title in ('manifest.json', 'manifest.sigstore.json')]
    return json.dumps(dict(artifactType=kind, layers=layers), sort_keys=True).encode()


def tags():
    return [path for path in registry.iterdir() if path.is_dir()] if registry.exists() else []


with open(os.environ['FAKE_LOG'], 'a') as stream:
    stream.write(json.dumps(dict(name=name, args=args, tmpdir=temporary, tmp_mode=mode, cwd=os.getcwd(),
                                 checksum=os.getenv('AWS_REQUEST_CHECKSUM_CALCULATION'), config_path=config,
                                 config=Path(config).read_text() if config and Path(config).exists() else '')) + '\n')
if os.getenv('FAKE_TERMINATE_ON') in args:
    os.kill(os.getppid(), signal.SIGTERM)
    time.sleep(5)
    sys.exit(1)
if os.getenv('FAKE_FAIL_ON') in args:
    sys.exit(1)
if name == 'aws':
    if '--version' in args:
        print('aws-cli/' + os.getenv('FAKE_AWS_VERSION', '2.40.0') + ' Python/synthetic')
        sys.exit(0)
    key = option('--key') if '--key' in args else ''
    filename = key.rsplit('/', 1)[-1]
    path = remote / option('--bucket') / key if key else None
    if 'list-objects-v2' in args:
        root = remote / option('--bucket') / option('--prefix')
        print(sum(item.is_file() for item in root.rglob('*')) if root.exists() else 0)
    elif 'head-object' in args:
        if not path.exists():
            sys.exit(254)
        data = path.read_bytes()
        size = table('FAKE_HEAD_SIZES').get(filename, len(data)) + (os.getenv('FAKE_BAD_SIZE') == filename)
        print(json.dumps(dict(ContentLength=size, ETag=table('FAKE_HEAD_ETAGS').get(filename, etag(data)))))
    elif 'put-object' in args:
        path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(option('--body'), path)
        print(json.dumps(dict(ETag=etag(path.read_bytes()))))
        replaced = os.getenv('FAKE_REPLACE_AFTER_UPLOAD')
        if replaced and filename == 'release.json':
            target = path.parent / replaced
            target.write_bytes(b'x' * target.stat().st_size)
    elif 'create-multipart-upload' in args:
        print(json.dumps(dict(UploadId='synthetic-upload')))
    elif 'upload-part' in args:
        part = remote / '.parts' / key / option('--part-number')
        part.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(option('--body'), part)
        print(json.dumps(dict(ETag=etag(part.read_bytes()))))
    elif 'complete-multipart-upload' in args:
        numbers = [item['PartNumber'] for item in json.loads(option('--multipart-upload'))['Parts']]
        data = b''.join((remote / '.parts' / key / str(number)).read_bytes() for number in numbers)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        print(json.dumps(dict(ETag=etag(data))))
    elif 'abort-multipart-upload' in args:
        print('{}')
    elif 'get-object' in args:
        data = path.read_bytes()
        if os.getenv('FAKE_GROW_AFTER_HEAD') == filename:
            data += b'growth'
        total = len(data)
        first, last = option('--range').removeprefix('bytes=').split('-')
        Path(args[-1]).write_bytes(data[int(first):int(last) + 1])
        result = dict(ETag=etag(data), ContentRange=f'bytes {first}-{last}/{total}')
        result.update(table('FAKE_GET_METADATA').get(filename, {}))
        print(json.dumps(result))
    else:
        sys.exit('unsupported synthetic aws call')
elif name == 'oras':
    if args[0] in ('login', 'logout'):
        sys.stdin.read() if args[0] == 'login' else None
    elif args[0] == 'resolve':
        if os.getenv('FAKE_RESOLVE_ERROR'):
            sys.stderr.write(os.environ['FAKE_RESOLVE_ERROR'] + '\n')
            sys.exit(1)
        directory = registry / args[1].rsplit(':', 1)[1]
        if not directory.is_dir():
            sys.stderr.write(f'Error response from registry: failed to resolve digest: {args[1]}: not found\n')
            sys.exit(1)
        print('sha256:' + sha(artifact(directory)))
    elif args[:2] == ['manifest', 'fetch']:
        wanted = args[2].rsplit('sha256:', 1)[1]
        sys.stdout.buffer.write(next(data for data in map(artifact, tags()) if sha(data) == wanted))
    elif args[:2] == ['blob', 'fetch']:
        wanted = args[-1].rsplit('sha256:', 1)[1]
        files = [path for directory in tags() for path in directory.iterdir()]
        sys.stdout.buffer.write(next(path.read_bytes() for path in files if sha(path.read_bytes()) == wanted))
    elif args[0] == 'push':
        directory = registry / args[1].rsplit(':', 1)[1]
        directory.mkdir(parents=True, exist_ok=True)
        for layer in args[2:]:
            title = layer.split(':', 1)[0]
            if title.startswith('manifest'):
                shutil.copyfile(title, directory / title)
    elif args[0] == 'tag':
        source = registry / args[1].rsplit(':', 1)[1]
        target = registry / args[2]
        shutil.rmtree(target, ignore_errors=True)
        shutil.copytree(source, target)
    else:
        sys.exit('unsupported synthetic oras call')
elif name == 'cosign':
    if os.getenv('FAKE_BAD_SIGNATURE') and args[0] == 'verify-blob':
        sys.exit(1)
    if args[0] == 'sign-blob':
        Path(option('--bundle')).write_bytes(b'{"synthetic":"bundle"}')
        print('SIGNATURE-ON-STDOUT')
elif name == 'gh':
    if os.getenv('FAKE_FAIL_DISPATCH'):
        sys.exit(1)
'''


def install_fakes(directory):
    directory.mkdir(parents=True, exist_ok=True)
    for name in ('aws', 'gh', 'oras', 'cosign'):
        path = directory / name
        path.write_text(FAKE)
        path.chmod(0o755)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def encoded(obj):
    return json.dumps(obj, sort_keys=True, ensure_ascii=False).encode()


def canonical(obj):
    return json.dumps(obj, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def step(name, **parameters):
    return dict(name=name, version=1, parameters=parameters)


def state(rel=REL):
    row = dict(kind='frameworks', rel=rel, schema_version=1,
               dense_recipe=dict(version=1, model='synthetic', dimensions=4, query_hash='e' * 64, document_hash='f' * 64),
               chunk_recipe=step('chunk', limit=512, schema='chunks/1', prefix_format_version=1),
               lexical_recipe=dict(version=1, k1=1.2, b=0.75),
               retrieval_recipe=dict(version=2, status='measured', abstention=dict(threshold=0.5)),
               snapshot_recipe=step('snapshot', public_floors='sources/frameworks-floors.yaml'),
               parse_recipe=step('parse', schema='controls/1'),
               relate_recipe=step('relate', schema='edges/1', forms=['<a>&b']),
               embed_recipe=step('embed', embedding_recipe_hash='d' * 64),
               framework_count=2, version_count=3, control_count=1612, versions_sealed=True,
               built_at='2099-01-02T00:00:00+00:00', extensions=dict(vector='0.8.1', ltree='1.3'))
    result = dict(row=row, dump=b'dump ' + SENTINEL.encode(),
                  build=dict(dump=dict(sha256='', size=0), recipe=step('build', loader_version=1, k1=1.2),
                             notes=SENTINEL),
                  evaluate=dict(recipe=step('evaluate', limit=8), inputs={}, outputs={}),
                  report=dict(version=1, rel=rel, quality_pass=True, previous=None, cases=[dict(question=SENTINEL)]))
    return bind(result)


def bind(value):
    from frameworks_recipes import recipe_sha256
    value['build']['dump'] = dict(sha256=digest(value['dump']), size=len(value['dump']))
    report = value['report']
    report['dump_sha256'] = digest(value['dump'])
    report['build_manifest_sha256'] = digest(encoded(value['build']))
    hashes = {name: recipe_sha256(json.loads(json.dumps(value['row'][name + '_recipe']), parse_int=float))
              for name in STEPS}
    hashes['build'] = recipe_sha256(json.loads(json.dumps(value['build']['recipe']), parse_int=float))
    hashes['evaluate'] = recipe_sha256(json.loads(json.dumps(value['evaluate']['recipe']), parse_int=float))
    report['recipe_hashes'] = hashes
    return value


def rebind_build(value):
    value['report']['build_manifest_sha256'] = digest(encoded(value['build']))
    return value


def object_bytes(value):
    return {'frameworks.dump': value['dump'], 'build.json': encoded(value['build']),
            'evaluate.json': encoded(value['evaluate']), 'report.json': encoded(value['report']),
            'release.json': encoded(value['row'])}


def write_run(base, value):
    for name, data in object_bytes(value).items():
        path = base / OBJECTS[name]
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)


def write_objects(directory, value):
    directory.mkdir(parents=True, exist_ok=True)
    paths = {}
    for name, data in object_bytes(value).items():
        paths[name] = directory / name
        paths[name].write_bytes(data)
    return paths


def manifest_object(value, sequence=3, commit=COMMIT, rel=None):
    row, report = value['row'], value['report']
    rel = rel or row['rel']
    previous = report['previous']['rel'] if report['previous'] else None
    data = object_bytes(value)
    return dict(kind='trachuan.corpus-release', version=1, corpus_kind='frameworks', repository='dannyota/trachuan',
                commit=commit, rel=rel, database='frameworks_' + rel, sequence=sequence, previous=previous,
                dump=dict(object=f'frameworks/{rel}/frameworks.dump', sha256=digest(value['dump']), size=len(value['dump'])),
                report=dict(sha256=digest(data['report.json']), quality_pass=True),
                build_artifact_sha256=digest(data['build.json']), schema_version=row['schema_version'],
                extensions=copy.deepcopy(row['extensions']),
                recipes=dict(dense=copy.deepcopy(row['dense_recipe']), chunk=copy.deepcopy(row['chunk_recipe']),
                             lexical=copy.deepcopy(row['lexical_recipe']), retrieval=copy.deepcopy(row['retrieval_recipe'])),
                counts=dict(versions=row['version_count'], controls=row['control_count']))


def publish_tag(registry, tag, manifest):
    directory = Path(registry) / tag
    directory.mkdir(parents=True, exist_ok=True)
    (directory / 'manifest.json').write_bytes(canonical(manifest))
    (directory / 'manifest.sigstore.json').write_bytes(b'{"synthetic":"previous bundle"}')


def calls(log):
    path = Path(log)
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
