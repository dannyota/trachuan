import hashlib
import json

from corpus_manifest import non_regression_contract


def digest(data):
    return hashlib.sha256(data).hexdigest()


def write_fixture(path):
    dump = b'synthetic corpus dump\n'
    manifest = (digest(dump) + '  corpus.dump\n').encode()
    recipes = dict(document={'model': 'synthetic', 'version': 1, 'dimensions': 128},
                   query={'model': 'synthetic', 'version': 1, 'dimensions': 128}, chunk={'version': 1},
                   lexical={'version': 1}, retrieval={'version': 3})
    inputs = {name: json.dumps(recipe, separators=(',', ':')) for name, recipe in recipes.items()}
    row = dict(country='vn', rel='20990102_1', schema_version=6,
               dense_recipe=dict(version=1, model='synthetic', dimensions=128,
                                 document=recipes['document'], query=recipes['query'],
                                 document_hash=digest(inputs['document'].encode()),
                                 query_hash=digest(inputs['query'].encode())),
               chunk_recipe=recipes['chunk'], lexical_recipe=recipes['lexical'],
               retrieval_recipe=recipes['retrieval'], recipe_hash_inputs=inputs,
               extensions={'vector': '0.8.1', 'ltree': '1.3'},
               run=dict(complete=True, expected_items=5, incomplete_items=0,
                        unaccepted_gaps=0, build_artifact_sha256=digest(manifest)))
    comparisons = [entry | {'status': 'pass'} for entry in non_regression_contract()['old_product']]
    report = dict(version=1, country=row['country'], rel=row['rel'], status='complete',
                  quality_pass=True, release_eligible=False, dump_sha256=digest(dump),
                  build_artifact_sha256=digest(manifest),
                  previous_diff={'previous': dict(country='vn', rel='20990101_1', report_sha256='b' * 64)},
                  recipes={name: digest(value.encode()) for name, value in inputs.items()},
                  non_regression=[item.copy() for item in comparisons],
                  cohorts={'legacy': {'case_count': 5},
                           'old_product': {'case_count': 5, 'non_regression': [item.copy() for item in comparisons]},
                           'development': {'case_count': 2, 'non_regression': [item.copy() for item in comparisons]}})
    (path / 'corpus.dump').write_bytes(dump)
    (path / 'MANIFEST.sha256').write_bytes(manifest)
    save_fixture(path, row, report)
    return row, report


def save_fixture(path, row, report):
    (path / 'release.json').write_text(json.dumps(row))
    (path / 'report.json').write_text(json.dumps(report))
