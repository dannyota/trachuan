import argparse
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from updater.deploy_io import atomic
from updater.deploy_verify import policy


def sync_directory(path):
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def file(source, target, policy_file=False, mode=0o644):
    source, target = Path(source), Path(target)
    if policy_file:
        policy(source)
    atomic(target, source.read_bytes(), mode)


def tree(path):
    path = Path(path)
    directories = [path]
    for child in path.rglob('*'):
        if child.is_dir():
            directories.append(child)
        else:
            with child.open('rb') as source:
                os.fsync(source.fileno())
    for directory in reversed(directories):
        sync_directory(directory)


def rename(source, target):
    os.replace(source, target)
    sync_directory(Path(target).parent)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('kind', choices=('policy', 'file', 'executable', 'tree', 'rename', 'directory'))
    parser.add_argument('source', type=Path)
    parser.add_argument('target', type=Path, nargs='?')
    args = parser.parse_args()
    if args.kind == 'tree':
        tree(args.source)
    elif args.kind == 'directory':
        sync_directory(args.source)
    elif args.kind == 'rename':
        rename(args.source, args.target)
    else:
        file(args.source, args.target, policy_file=args.kind == 'policy',
             mode=0o755 if args.kind == 'executable' else 0o644)


if __name__ == '__main__':
    main()
