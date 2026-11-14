import json
import os
import time
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from manifest import require_match


def fetch_runs(commit):
    query = urlencode(dict(branch='main', event='push', head_sha=commit, per_page=100))
    request = Request('https://api.github.com/repos/dannyota/trachuan/actions/workflows/ci.yml/runs?' + query,
                      headers={'Authorization': 'Bearer ' + os.environ['GITHUB_TOKEN'],
                               'Accept': 'application/vnd.github+json',
                               'X-GitHub-Api-Version': '2026-03-10'})
    with urlopen(request, timeout=30) as response:
        return json.load(response)['workflow_runs']


def wait(commit, fetch=fetch_runs, clock=time.monotonic, sleep=time.sleep, timeout=3600):
    require_match(r'[0-9a-f]{40}', commit, 'commit')
    deadline = clock() + timeout
    while True:
        runs = [run for run in fetch(commit)
                if run['head_sha'] == commit and run['head_branch'] == 'main' and run['event'] == 'push']
        if clock() > deadline:
            raise TimeoutError('main CI did not succeed within 60 minutes')
        if runs:
            run = max(runs, key=lambda run: run['id'])
            if run['status'] == 'completed':
                if run['conclusion'] != 'success':
                    raise ValueError(f"main CI concluded {run['conclusion']}")
                return
        remaining = deadline - clock()
        if remaining <= 0:
            raise TimeoutError('main CI did not succeed within 60 minutes')
        sleep(min(15, remaining))


if __name__ == '__main__':
    wait(os.environ['GITHUB_SHA'])
