import argparse
from pathlib import Path
import subprocess
import tempfile

from corpus_manifest import decode, identity
from manifest import positive
from object_store import ObjectReader, require_free_space, require_pinned_cli, s3_endpoint


def download(country, rel, directory):
    identity(country, rel)
    endpoint = s3_endpoint()
    require_pinned_cli()
    directory.mkdir(parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory() as temporary:
        reader = ObjectReader('trachuan-releases', endpoint, temporary)
        limits = {'corpus.dump': 16 * 1024**3, 'MANIFEST.sha256': 1024**2,
                  'report.json': 16 * 1024**2, 'release.json': 16 * 1024**2,
                  'build.json': 16 * 1024**2}
        objects = {}
        for name, limit in limits.items():
            try:
                objects[name] = reader.head(f'{country}/{rel}/{name}', name, limit)
            except subprocess.CalledProcessError:
                if name == 'build.json':
                    continue
                raise
        require_free_space(objects, directory, 'corpus')

        def fetch(name):
            expected, etag = objects[name]
            reader.get(f'{country}/{rel}/{name}', name, directory / name, expected, etag)

        fetch('release.json')
        row = decode((directory / 'release.json').read_bytes())
        if not isinstance(row, dict) or row.get('country') != country or row.get('rel') != rel:
            raise ValueError('downloaded release identity differs from request')
        positive(row.get('schema_version'), 'schema version')
        if row['schema_version'] >= 8 and 'build.json' not in objects:
            raise ValueError('schema 8 requires build.json HEAD metadata')
        for name in objects:
            if name == 'release.json' or (name == 'build.json' and row['schema_version'] < 8):
                continue
            fetch(name)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    download(args.country, args.rel, args.directory)


if __name__ == '__main__':
    main()
