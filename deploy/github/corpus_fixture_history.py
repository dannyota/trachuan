import hashlib
import json
import os
from pathlib import Path
import subprocess

DEVELOPMENT = 'apps/corpus/internal/evaluation/testdata/vn/development.json'
REPOSITORY = Path(__file__).resolve().parents[2]
CASE_FIELDS = {'tags', 'id', 'question', 'expected_citations', 'expect_abstain', 'expect_fail', 'notes'}
CITATION_FIELDS = {'doc_number', 'article', 'clause', 'point', 'alt_doc_numbers', 'relation_ok'}
TAGS = {'identifier', 'clause', 'multi_document', 'abbreviation', 'missing_date', 'historical',
        'transition', 'out_of_domain', 'negative'}


def git(root, *args, data=None):
    env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
    result = subprocess.run(['git', '-C', str(root), *args], input=data, capture_output=True, env=env)
    if result.returncode != 0:
        raise ValueError(f'git {args[0]} failed in {root}')
    return result.stdout


def blobs(output, count, name):
    out, offset = [], 0
    for _ in range(count):
        end = output.index(b'\n', offset)
        header = output[offset:end].split(b' ')
        offset = end + 1
        if header[-1] == b'missing':
            out.append(None)
            continue
        if len(header) != 3 or header[1] != b'blob':
            raise ValueError(f'{name} fixture path is not a file in main history')
        size = int(header[2])
        out.append(output[offset:offset + size])
        offset += size + 1
    if offset != len(output):
        raise ValueError('unexpected git cat-file output')
    return out


class FixtureHistory:
    def __init__(self, root=REPOSITORY, path=DEVELOPMENT):
        self.root, self.path, self.versions = Path(root), path, None
        self.name = Path(path).stem

    def load(self):
        if self.versions is not None:
            return self.versions
        if git(self.root, 'rev-parse', '--is-shallow-repository').strip() != b'false':
            raise ValueError('fixture history needs a full clone, not a shallow repository')
        try:
            git(self.root, 'rev-parse', '--verify', '--quiet', 'refs/heads/main^{commit}')
        except ValueError:
            raise ValueError('fixture history needs the local main branch') from None
        commits = git(self.root, 'rev-list', '--first-parent', 'refs/heads/main', '--', self.path).split()
        query = b''.join(commit + b':' + self.path.encode() + b'\n' for commit in commits)
        output = git(self.root, 'cat-file', '--batch', data=query) if commits else b''
        versions = {}
        for position, data in enumerate(blobs(output, len(commits), self.name)):
            if data is not None:
                versions.setdefault(hashlib.sha256(data).hexdigest(), (position, data))
        self.versions = versions
        return versions

    def version(self, digest):
        versions = self.load()
        if digest not in versions:
            raise ValueError(f'{self.name} fixture {digest} is not in the history of the local main branch; '
                             'update main to include it and retry')
        return versions[digest]

    def fixture(self, digest):
        return self.version(digest)[1]

    def require_order(self, candidate, previous):
        if self.version(candidate)[0] > self.version(previous)[0]:
            raise ValueError('candidate development fixture is older in main history than the previous report fixture')


def unique_object(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError('duplicate JSON field in fixture')
        obj[key] = value
    return obj


def text(value, field, required=False):
    if value is None and not required:
        return ''
    if not isinstance(value, str) or (required and not value.strip()):
        raise ValueError('invalid development fixture ' + field)
    return value


def development_cases(data):
    def invalid_constant(value):
        raise ValueError('invalid JSON constant in development fixture')
    cases = json.loads(data, object_pairs_hook=unique_object, parse_constant=invalid_constant)
    if not isinstance(cases, list):
        raise ValueError('development fixture is not a list')
    seen = set()
    for case in cases:
        if not isinstance(case, dict) or not set(case) <= CASE_FIELDS:
            raise ValueError('invalid development fixture case fields')
        case_id = text(case.get('id'), 'id', True)
        text(case.get('question'), 'question', True)
        tags = case.get('tags')
        if case_id in seen or not isinstance(tags, list) or not tags or len(set(tags)) != len(tags) or \
                any(not isinstance(tag, str) or tag not in TAGS for tag in tags):
            raise ValueError('invalid development fixture id or tags: ' + case_id)
        seen.add(case_id)
        if case.get('expect_abstain') is not None and type(case['expect_abstain']) is not bool:
            raise ValueError('invalid development fixture expect_abstain: ' + case_id)
        citations = case.get('expected_citations')
        if citations is not None and not isinstance(citations, list):
            raise ValueError('invalid development fixture citations: ' + case_id)
        for citation in citations or ():
            if not isinstance(citation, dict) or not set(citation) <= CITATION_FIELDS:
                raise ValueError('invalid development fixture citation fields: ' + case_id)
            for field in ('doc_number', 'article', 'clause', 'point'):
                text(citation.get(field), field)
            numbers = citation.get('alt_doc_numbers')
            if numbers is not None and (not isinstance(numbers, list) or any(not isinstance(n, str) for n in numbers)):
                raise ValueError('invalid development fixture alternate numbers: ' + case_id)
            if citation.get('relation_ok') is not None and type(citation['relation_ok']) is not bool:
                raise ValueError('invalid development fixture relation_ok: ' + case_id)
    return cases


def cohort_of(case):
    return 'development_period' if 'historical' in case['tags'] else 'development'


def content(case):
    citations = tuple((text(c.get('doc_number'), 'doc_number'), text(c.get('article'), 'article'),
                       text(c.get('clause'), 'clause'), text(c.get('point'), 'point'),
                       tuple(c.get('alt_doc_numbers') or ()), bool(c.get('relation_ok')))
                      for c in case.get('expected_citations') or ())
    return case['question'], citations, bool(case.get('expect_abstain'))
