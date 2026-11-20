from repositories import require_listed


def dispatch_command(repository, country, rel, report_sha256, release_sha256):
    command = ['gh', 'workflow', 'run', 'corpus-release.yml', '--repo', require_listed(repository), '--ref', 'main',
               '-f', f'country={country}', '-f', f'rel={rel}', '-f', f'report_sha256={report_sha256}']
    return command + ['-f', f'release_sha256={release_sha256}'] if release_sha256 else command
