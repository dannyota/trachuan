def dispatch_command(country, rel, report_sha256, release_sha256):
    return ['gh', 'workflow', 'run', 'corpus-release.yml', '--ref', 'main', '-f', f'country={country}',
            '-f', f'rel={rel}', '-f', f'report_sha256={report_sha256}', '-f', f'release_sha256={release_sha256}']
