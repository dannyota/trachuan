import json
import os
from pathlib import Path
import re
import subprocess
import sys


KEY = re.compile(r'^(go-[^-]+-[^-]+-(?:static|integration-(?:integration|proof)|e2e|queryembed))-([0-9a-f]{64})-([0-9a-f]{40})$')


def pages(text):
    decoder = json.JSONDecoder()
    while text.strip():
        page, end = decoder.raw_decode(text.lstrip())
        yield from page['actions_caches']
        text = text.lstrip()[end:]


def select(caches):
    groups = {}
    for cache in {cache['id']: cache for cache in caches}.values():
        match = KEY.fullmatch(cache['key'])
        if match:
            groups.setdefault((match.group(1), cache['ref']), []).append(cache)
    return [cache['id'] for group in groups.values()
            for cache in sorted(group, key=lambda c: (c['created_at'], c['id']), reverse=True)[1:]]


def listing(endpoint):
    result = subprocess.run(['gh', 'api', '--paginate', endpoint], check=True, capture_output=True, text=True)
    return list(pages(result.stdout))


def report(label, caches):
    print(f"{label}: {len(caches)} caches, {sum(cache['size_in_bytes'] for cache in caches)} bytes", flush=True)


def main():
    if len(sys.argv) == 3 and sys.argv[1] == 'select':
        print(*select(list(pages(Path(sys.argv[2]).read_text()))), sep='\n')
        return
    endpoint = f"repos/{os.environ['GITHUB_REPOSITORY']}/actions/caches"
    caches = listing(endpoint)
    report('Before', caches)
    for cache_id in select(caches):
        subprocess.run(['gh', 'api', '--method', 'DELETE', f'{endpoint}/{cache_id}'], check=True)
    report('After', listing(endpoint))


if __name__ == '__main__':
    main()
