import argparse
from pathlib import Path
import tempfile

from corpus_manifest import sha256
from frameworks_cli import guarded
from frameworks_release import BUCKET, LIMITS, object_key, release_id, report_id
from object_store import ObjectReader, require_free_space, require_pinned_cli, s3_endpoint


def download(rel, report_sha256, directory):
    release_id(rel)
    report_id(report_sha256)
    endpoint = s3_endpoint()
    require_pinned_cli()
    directory.mkdir(mode=0o700, parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory() as temporary:
        reader = ObjectReader(BUCKET, endpoint, temporary)
        objects = {name: reader.head(object_key(rel, name), name, limit) for name, limit in LIMITS.items()}
        require_free_space(objects, directory, 'frameworks')
        for name in ('report.json', *(item for item in objects if item != 'report.json')):
            expected, etag = objects[name]
            reader.get(object_key(rel, name), name, directory / name, expected, etag)
            if name == 'report.json' and sha256(directory / name) != report_sha256:
                raise ValueError('report differs from the dispatched SHA-256')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('rel')
    parser.add_argument('report_sha256')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    guarded(lambda: download(args.rel, args.report_sha256, args.directory))


if __name__ == '__main__':
    main()
