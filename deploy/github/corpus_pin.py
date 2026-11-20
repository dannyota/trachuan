import argparse
from pathlib import Path
import tempfile

from corpus_manifest import identity
from corpus_registry import existing_release
from manifest import require_match


def require_pinned_or_signed(country, rel, release_sha256):
    identity(country, rel)
    if release_sha256 != '':
        require_match(r'[0-9a-f]{64}', release_sha256, 'release SHA-256')
        return
    with tempfile.TemporaryDirectory() as temporary:
        if existing_release(country, rel, Path(temporary)) is None:
            raise ValueError(f'an empty release SHA-256 signs only an already signed {country}-{rel}')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('release_sha256')
    args = parser.parse_args()
    require_pinned_or_signed(args.country, args.rel, args.release_sha256)


if __name__ == '__main__':
    main()
