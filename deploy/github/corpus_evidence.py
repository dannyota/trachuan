import hashlib
import json

from manifest import require_match

FINDINGS, IDENTITY, CHANGES = 'findings.json', 'identity.jsonl', 'changes.jsonl'
SIDECARS = (FINDINGS, IDENTITY, CHANGES)
RUN_FIELDS = {FINDINGS: 'findings_sha256', IDENTITY: 'identity_sha256', CHANGES: 'changes_sha256'}
LIMITS = {FINDINGS: 16 * 1024**2, IDENTITY: 1024**3, CHANGES: 1024**3}
LINE_LIMIT = 1024**2
FINDINGS_SCHEMA = 2
SUMMARY_SCHEMA = 1
SEVERITIES = ('blocking', 'review', 'info')
IDENTITY_TYPES = ('document', 'provision', 'relation')
CHANGE_TYPES = ('document', 'document_key', 'document_legal', 'document_metadata', 'document_provenance',
                'provision', 'provision_classification', 'provision_validity', 'relation')
FINDINGS_KEYS = {'schema', 'bindings', 'detectors', 'findings', 'new', 'gone', 'carried', 'unmatched'}
DETECTOR_KEYS = {'complete', 'severity', 'observed', 'not_observable', 'excluded'}


def prefixed(digest):
    return 'sha256:' + digest


def bare_or_none(value, name):
    if value == 'none':
        return value
    require_match(r'sha256:[0-9a-f]{64}', value, name)
    return value.removeprefix('sha256:')


def decoded(data):
    from corpus_manifest import decode
    return decode(data)


def check_detectors(detectors):
    if not isinstance(detectors, dict):
        raise ValueError('invalid findings detectors')
    for name, entry in detectors.items():
        if not isinstance(entry, dict) or set(entry) != DETECTOR_KEYS:
            raise ValueError('invalid detector entry: ' + name)
        if entry['complete'] is not True:
            raise ValueError('detector not complete: ' + name)
        if entry['severity'] not in SEVERITIES:
            raise ValueError('invalid detector severity: ' + name)
        if any(type(entry[field]) is not int or entry[field] < 0 for field in ('observed', 'not_observable', 'excluded')):
            raise ValueError('invalid detector counts: ' + name)


def check_findings(findings, base, previous_rel):
    if not isinstance(findings, dict) or set(findings) != FINDINGS_KEYS:
        raise ValueError('invalid findings fields')
    if type(findings['schema']) is not int or findings['schema'] != FINDINGS_SCHEMA:
        raise ValueError('unsupported findings schema')
    bindings = findings['bindings']
    if not isinstance(bindings, dict) or set(bindings) != set(base) | {'detector_registry_sha256', 'predecessor_rel', 'predecessor_findings_sha256'}:
        raise ValueError('invalid findings bindings')
    for key, value in base.items():
        if bindings[key] != value:
            raise ValueError('findings binding differs from the release: ' + key)
    require_match(r'sha256:[0-9a-f]{64}', bindings['detector_registry_sha256'], 'detector registry hash')
    if bindings['predecessor_rel'] != previous_rel:
        raise ValueError("findings predecessor differs from the report's previous_diff")
    predecessor = bare_or_none(bindings['predecessor_findings_sha256'], 'predecessor findings hash')
    detectors = findings['detectors']
    check_detectors(detectors)
    if any(not isinstance(findings[key], list) for key in ('findings', 'new', 'gone', 'carried', 'unmatched')):
        raise ValueError('invalid findings lists')
    counts = dict(review=0, info=0)
    for finding in findings['findings']:
        if not isinstance(finding, dict) or not isinstance(finding.get('detector'), str) or finding.get('status') not in ('open', 'accepted'):
            raise ValueError('invalid finding')
        if finding['status'] != 'open':
            continue
        entry = detectors.get(finding['detector'])
        if entry is None or entry['severity'] == 'blocking':
            raise ValueError('open finding of a blocking or unlisted detector: ' + str(finding.get('key')))
        counts[entry['severity']] += 1
    return predecessor, counts, dict(new=len(findings['new']), gone=len(findings['gone']))


def compact(record):
    text = json.dumps(record, ensure_ascii=False, separators=(',', ':'))
    return text.replace('\u2028', '\\u2028').replace('\u2029', '\\u2029').encode()


def relation_key(record, body):
    tail = ['change', 'count'] if 'change' in record else ['count']
    if list(record)[-len(tail):] != tail or type(record['count']) is not int or record['count'] < 1:
        raise ValueError('relation record must end with its change and a positive count')
    if 'change' in record and record['change'] not in ('added', 'removed'):
        raise ValueError('invalid relation change')
    key = compact({name: value for name, value in record.items() if name not in tail})[:-1]
    if not body.startswith(key) or not all(isinstance(record.get(name), str) for name in ('kind', 'method')):
        raise ValueError('invalid relation record')
    return key


def order_key(record, body):
    kind = record['type']
    if kind == 'relation':
        return relation_key(record, body)
    fields = ('key', 'anchor') if kind.startswith('provision') else ('key',)
    if any(not isinstance(record.get(name), str) for name in fields):
        raise ValueError('invalid ' + kind + ' record')
    return tuple(record[name] for name in fields)


def scan_lines(path, types, check_header):
    digest, header, records, last = hashlib.sha256(), None, 0, ('', None)
    with path.open('rb') as stream:
        while line := stream.readline(LINE_LIMIT + 2):
            digest.update(line)
            if len(line.rstrip(b'\n')) > LINE_LIMIT:
                raise ValueError(path.name + ' line exceeds 1 MiB')
            if not line.endswith(b'\n'):
                raise ValueError(path.name + ' does not end its last line')
            body = line[:-1]
            record = decoded(body)
            if compact(record) != body:
                raise ValueError(path.name + ' line is not compact JSON')
            if header is None:
                header = check_header(record)
                continue
            if not isinstance(record, dict) or record.get('type') not in types:
                raise ValueError(path.name + ' holds an unknown record type')
            key = (record['type'], order_key(record, body))
            if key[0] < last[0] or (key[0] == last[0] and key[1] <= last[1]):
                raise ValueError(path.name + ' records are out of order')
            last, records = key, records + 1
    if header is None:
        raise ValueError(path.name + ' has no header')
    return digest.hexdigest(), header, records


def summary_header(record, keys):
    if not isinstance(record, dict) or set(record) != keys:
        raise ValueError('invalid summary header')
    if type(record['schema']) is not int or record['schema'] != SUMMARY_SCHEMA:
        raise ValueError('unsupported summary schema')
    return record


def identity_header(base):
    def check(record):
        summary_header(record, {'schema', 'bindings'})
        if record['bindings'] != base:
            raise ValueError('identity bindings differ from the release')
        return record
    return check


def changes_header(base, previous_rel):
    def check(record):
        summary_header(record, {'schema', 'bindings', 'baseline'})
        bindings = record['bindings']
        if not isinstance(bindings, dict) or set(bindings) != set(base) | {'predecessor_rel', 'predecessor_identity_sha256'}:
            raise ValueError('invalid changes bindings')
        if any(bindings[key] != value for key, value in base.items()):
            raise ValueError('changes bindings differ from the release')
        if bindings['predecessor_rel'] != previous_rel:
            raise ValueError("changes predecessor differs from the report's previous_diff")
        predecessor = bare_or_none(bindings['predecessor_identity_sha256'], 'predecessor identity hash')
        if record['baseline'] is not (predecessor == 'none'):
            raise ValueError('changes baseline disagrees with its predecessor')
        return predecessor
    return check


def validate_sidecars(row, report, report_bytes, directory, dump_hash, build_hash):
    run = row['run']
    for name in SIDECARS:
        require_match(r'[0-9a-f]{64}', run.get(RUN_FIELDS[name]), 'release run ' + RUN_FIELDS[name])
        path = directory / name
        if not path.is_file():
            raise ValueError('evidence file missing: ' + name)
        if path.stat().st_size > LIMITS[name]:
            raise ValueError(name + ' exceeds its size limit')
    previous_rel = report['previous_diff']['previous']['rel']
    base = dict(rel=row['rel'], dump_sha256=prefixed(dump_hash), build_artifact_sha256=prefixed(build_hash),
                report_sha256=prefixed(hashlib.sha256(report_bytes).hexdigest()))
    data = (directory / FINDINGS).read_bytes()
    digests = {FINDINGS: hashlib.sha256(data).hexdigest()}
    predecessor_findings, counts, changed = check_findings(decoded(data), base, previous_rel)
    digests[IDENTITY], _, _ = scan_lines(directory / IDENTITY, IDENTITY_TYPES, identity_header(base))
    digests[CHANGES], predecessor_identity, records = scan_lines(directory / CHANGES, CHANGE_TYPES, changes_header(base, previous_rel))
    if predecessor_identity == 'none' and records:
        raise ValueError('baseline changes list records')
    for name, digest in digests.items():
        if run[RUN_FIELDS[name]] != digest:
            raise ValueError(name + ' differs from release.json')
    return dict(evidence={RUN_FIELDS[name]: digests[name] for name in SIDECARS},
                predecessor=dict(rel=previous_rel, findings_sha256=predecessor_findings, identity_sha256=predecessor_identity),
                open=counts, changed=changed)


def require_predecessor(sidecars, manifest):
    evidence = manifest.get('evidence')
    expected = ('none', 'none') if evidence is None else (evidence['findings_sha256'], evidence['identity_sha256'])
    predecessor = sidecars['predecessor']
    if (predecessor['findings_sha256'], predecessor['identity_sha256']) != expected:
        raise ValueError('evidence predecessor differs from the previous signed manifest')


def validate_evidence_field(value):
    if not isinstance(value, dict) or set(value) != set(RUN_FIELDS.values()):
        raise ValueError('invalid evidence fields')
    for field, digest in value.items():
        require_match(r'[0-9a-f]{64}', digest, 'evidence ' + field)
