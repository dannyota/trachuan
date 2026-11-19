def dispatch_command(repository, country, rel, report_sha256, release_sha256):
    command = ['gh', 'workflow', 'run', 'corpus-release.yml', '--repo', repository, '--ref', 'main',
               '-f', f'country={country}', '-f', f'rel={rel}', '-f', f'report_sha256={report_sha256}']
    return command + ['-f', f'release_sha256={release_sha256}'] if release_sha256 else command
