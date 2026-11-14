import subprocess

from manifest import require_match


def verify(path, bundle, workflow, ref, commit, release=False):
    require_match(r'[0-9a-f]{40}', commit, 'commit')
    if release:
        require_match(r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+', ref, 'release ref')
        identity = ['--certificate-identity-regexp',
                    r'^https://github\.com/dannyota/trachuan/\.github/workflows/images\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$']
    else:
        if workflow not in ('images.yml', 'rollback.yml'):
            raise ValueError('invalid workflow')
        pattern = r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+' if workflow == 'images.yml' else r'refs/heads/main'
        require_match(pattern, ref, 'workflow ref')
        identity = ['--certificate-identity',
                    f'https://github.com/dannyota/trachuan/.github/workflows/{workflow}@{ref}']
    subprocess.run(['cosign', 'verify-blob', '--bundle', str(bundle), *identity,
                    '--certificate-oidc-issuer', 'https://token.actions.githubusercontent.com',
                    '--certificate-github-workflow-ref', ref,
                    '--certificate-github-workflow-sha', commit, str(path)], check=True)


def sign(path, bundle, workflow, ref, commit):
    subprocess.run(['cosign', 'sign-blob', '--yes', '--bundle', str(bundle), str(path)], check=True)
    verify(path, bundle, workflow, ref, commit)
