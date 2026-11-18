import math
import re


def text(value):
    return isinstance(value, str) and re.fullmatch(r'[ -~]{1,200}', value) is not None


def hash64(value):
    return isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) is not None


def commit40(value):
    return isinstance(value, str) and re.fullmatch(r'[0-9a-f]{40}', value) is not None


def integer(value):
    return type(value) is int and value >= 0


def number(value):
    return type(value) in (int, float) and math.isfinite(value)


def boolean(value):
    return type(value) is bool


def exact(expected):
    return lambda value: type(value) is type(expected) and value == expected


EMBEDDING = {'model': text, 'revision': text, 'resolved_revision': commit40, 'tokenizer_revision': commit40,
             'tokenizer_sha256': hash64, 'weights_sha256': hash64, 'dimensions': integer, 'pooling': text,
             'normalization': text, 'input': text, 'runtime': text, 'backend': text, 'precision': text,
             'token_cap': integer, 'special_tokens': boolean, 'truncation': boolean, 'language': text,
             'tokenizer_library': text, 'runtime_library': text}
DENSE_1 = {'version': exact(1), 'model': text, 'dimensions': integer, 'document': EMBEDDING, 'document_hash': hash64,
           'query': EMBEDDING, 'query_hash': hash64}
CHUNK_1 = {'name': exact('chunk'), 'version': exact(1),
           'parameters': {'limit': integer, 'prefix_format_version': integer, 'schema': exact('chunks/1')}}
LEXICAL_1 = {'version': exact(1), 'algorithm': text, 'input': text, 'normalization': text, 'tokenizer': text, 'hash': text,
             'dimensions': integer, 'k1': number, 'b': number, 'query': text, 'collision': text}
RETRIEVAL_2 = {'version': exact(2), 'status': exact('measured'), 'dense_depth': integer, 'lexical_depth': integer,
               'rrf': {'k': integer, 'dense_weight': number, 'lexical_weight': number, 'routed_lexical_weight': number},
               'abstention': {'statistic': text, 'threshold': number, 'status': exact('measured'),
                              'calibration_sha256': hash64},
               'source': {'release': text, 'recipe_version': integer, 'calibration_sha256': hash64}}
VERSIONS = {'dense': {1: DENSE_1}, 'chunk': {1: CHUNK_1}, 'lexical': {1: LEXICAL_1}, 'retrieval': {2: RETRIEVAL_2}}


def check(value, shape, path):
    if isinstance(shape, dict):
        if not isinstance(value, dict) or set(value) != set(shape):
            raise ValueError('invalid frameworks recipe fields: ' + path)
        for key, item in shape.items():
            check(value[key], item, path + '.' + key)
    elif not shape(value):
        raise ValueError('invalid frameworks recipe value: ' + path)


def check_recipe_shapes(recipes):
    if not isinstance(recipes, dict) or set(recipes) != set(VERSIONS):
        raise ValueError('invalid frameworks recipe set')
    for name, shapes in VERSIONS.items():
        recipe = recipes[name]
        version = recipe.get('version') if isinstance(recipe, dict) else None
        if type(version) is not int or version not in shapes:
            raise ValueError('unknown frameworks recipe version: ' + name)
        check(recipe, shapes[version], name)
