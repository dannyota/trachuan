import argparse
import json
from pathlib import Path
import subprocess


def apply(config, request, dry_run=False):
    base = 'repos/dannyota/trachuan'
    existing = request('GET', base + '/rulesets?includes_parents=false&per_page=100')
    for ruleset in config['rulesets']:
        matches = [item for item in existing if item['name'] == ruleset['name']]
        if len(matches) > 1:
            raise ValueError('duplicate ruleset name: ' + ruleset['name'])
        if matches:
            request('PUT', base + '/rulesets/' + str(matches[0]['id']), ruleset)
        else:
            request('POST', base + '/rulesets', ruleset)
    environments = request('GET', base + '/environments?per_page=100')
    for environment in config['environments']:
        path = base + '/environments/' + environment['name']
        existing_env = next((item for item in environments if item['name'] == environment['name']), {})
        request('PUT', path, environment['settings'])
        custom = (existing_env.get('deployment_branch_policy') or {}).get('custom_branch_policies', False)
        policies = request('GET', path + '/deployment-branch-policies?per_page=100') if not dry_run or custom else []
        for policy in environment['refs']:
            if not any(all(item.get(key) == value for key, value in policy.items()) for item in policies):
                request('POST', path + '/deployment-branch-policies', policy)
        for policy in policies:
            if {'name': policy['name'], 'type': policy['type']} not in environment['refs']:
                request('DELETE', path + '/deployment-branch-policies/' + str(policy['id']))


def gh_request(method, path, body=None, dry_run=False):
    print(method + ' ' + path + (' ' + json.dumps(body, sort_keys=True) if body is not None else ''), flush=True)
    if dry_run and method != 'GET':
        return {}
    command = ['gh', 'api', '--method', method, path, '-H', 'Accept: application/vnd.github+json',
               '-H', 'X-GitHub-Api-Version: 2026-03-10']
    if method == 'GET':
        command += ['--paginate', '--slurp']
    if body is not None:
        command += ['--input', '-']
    result = subprocess.run(command, input=json.dumps(body) if body is not None else None,
                            text=True, stdout=subprocess.PIPE, check=True)
    if not result.stdout.strip():
        return {}
    data = json.loads(result.stdout)
    if method == 'GET':
        key = 'environments' if '/environments?' in path else 'branch_policies'
        return [item for page in data for item in (page if isinstance(page, list) else page[key])]
    return data


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    config = json.loads(Path(__file__).with_name('settings.json').read_text())
    apply(config, lambda method, path, body=None: gh_request(method, path, body, args.dry_run), args.dry_run)


if __name__ == '__main__':
    main()
