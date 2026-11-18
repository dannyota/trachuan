import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from corpus_evidence import require_predecessor
from corpus_manifest import decode, validate_report
from corpus_rules import has_decision
from manifest import canonical
from object_store import fetch_checked


def verified_previous(report, directory, client, release_row=None, sidecars=None):
    from corpus_registry import release

    previous = report['previous_diff']['previous']
    country, rel = previous['country'], previous['rel']
    manifest = release(country, rel, directory / 'previous-manifest')
    if manifest['country'] != country or manifest['rel'] != rel:
        raise ValueError('previous manifest identity differs from report')
    if release_row is not None and has_decision(release_row.get('run') or {}):
        if sidecars is None:
            raise ValueError('a release with a decision needs its evidence checked against the previous manifest')
        require_predecessor(sidecars, manifest)
    data = fetch_checked(client, 'trachuan-releases', f'{country}/{rel}/report.json',
                         directory / 'previous-report.json', 16 * 1024**2, 'previous report')
    digest = hashlib.sha256(data).hexdigest()
    if digest != previous['report_sha256'] or digest != manifest['report']['sha256']:
        raise ValueError('previous report SHA-256 differs from report or signed manifest')
    prior = decode(data)
    recipe = manifest['recipes']['retrieval']
    if not isinstance(recipe, dict):
        raise ValueError('previous signed retrieval recipe missing')
    if prior.get('version') == 2 and not isinstance(prior.get('retrieval_recipe'), dict):
        raise ValueError('previous version 2 retrieval recipe missing')
    if 'retrieval_recipe' in prior and canonical(prior['retrieval_recipe']) != canonical(recipe):
        raise ValueError('previous report retrieval recipe differs from signed manifest')
    prior = prior | {'retrieval_recipe': recipe}
    validate_report(report, report['country'], report['rel'], previous_report=prior, release_row=release_row)
    return prior


def remote_previous(report, directory, release_row=None, sidecars=None):
    endpoint = os.environ['OCI_S3_ENDPOINT']
    if not endpoint.startswith('https://') or '@' in endpoint:
        raise ValueError('OCI_S3_ENDPOINT must be HTTPS without credentials')
    env = dict(os.environ, AWS_REQUEST_CHECKSUM_CALCULATION='when_required',
               AWS_DEFAULT_REGION='ap-singapore-1', AWS_REGION='ap-singapore-1')
    command = ['aws']
    config = directory / 's3-config'
    config.write_text('[default]\nregion = ap-singapore-1\ns3 =\n'
                      '    addressing_style = path\n    signature_version = s3v4\n')
    env['AWS_CONFIG_FILE'] = str(config)
    command += ['--endpoint-url', endpoint, '--no-cli-pager', 's3api']

    def client(args):
        result = subprocess.run([*command, '--output', 'json', *args], check=True,
                                capture_output=True, text=True, env=env)
        return json.loads(result.stdout)

    return verified_previous(report, directory, client, release_row=release_row, sidecars=sidecars)


def check_remote(report, release_row=None, sidecars=None):
    with tempfile.TemporaryDirectory() as temporary:
        return remote_previous(report, Path(temporary), release_row=release_row, sidecars=sidecars)
