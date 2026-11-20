import re
import subprocess

from manifest import require_match
from repositories import require_listed


def verify(path, bundle, repository, workflow, ref, commit, release=False):
    require_listed(repository)
    require_match(r'[0-9a-f]{40}', commit, 'commit')
    if release:
        require_match(r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+', ref, 'release ref')
        identity = ['--certificate-identity-regexp', '^' + re.escape('https://github.com/' + repository)
                    + r'/\.github/workflows/images\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$']
    else:
        if workflow not in ('images.yml', 'rollback.yml', 'corpus-release.yml', 'frameworks-release.yml'):
            raise ValueError('invalid workflow')
        pattern = r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+' if workflow == 'images.yml' else r'refs/heads/main'
        require_match(pattern, ref, 'workflow ref')
        identity = ['--certificate-identity',
                    f'https://github.com/{repository}/.github/workflows/{workflow}@{ref}']
    subprocess.run(['cosign', 'verify-blob', '--bundle', str(bundle), *identity,
                    '--certificate-oidc-issuer', 'https://token.actions.githubusercontent.com',
                    '--certificate-github-workflow-repository', repository,
                    '--certificate-github-workflow-ref', ref,
                    '--certificate-github-workflow-sha', commit, str(path)], check=True)


def sign(path, bundle, repository, workflow, ref, commit, stdout=None):
    require_listed(repository)
    subprocess.run(['cosign', 'sign-blob', '--yes', '--bundle', str(bundle), str(path)], check=True, stdout=stdout)
    verify(path, bundle, repository, workflow, ref, commit)
