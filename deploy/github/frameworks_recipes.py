from decimal import Decimal
import hashlib
import json
import math

from corpus_manifest import unique_object

ESCAPES = {'"': '\\"', '\\': '\\\\', '\b': '\\b', '\f': '\\f', '\n': '\\n', '\r': '\\r', '\t': '\\t',
           '<': '\\u003c', '>': '\\u003e', '&': '\\u0026', ' ': '\\u2028', ' ': '\\u2029'}


def invalid_constant(value):
    raise ValueError('invalid JSON constant')


def parse(data, **options):
    try:
        return json.loads(data.decode('utf-8'), object_pairs_hook=unique_object,
                          parse_constant=invalid_constant, **options)
    except RecursionError as error:
        raise ValueError('JSON nesting too deep') from error


def decode(data):
    return parse(data)


def decode_floats(data):
    return parse(data, parse_int=float)


def go_string(value):
    value.encode('utf-8')
    out = []
    for char in value:
        if char in ESCAPES:
            out.append(ESCAPES[char])
        elif char < ' ':
            out.append(f'\\u{ord(char):04x}')
        else:
            out.append(char)
    return '"' + ''.join(out) + '"'


def go_number(value):
    try:
        number = float(value)
    except OverflowError as error:
        raise ValueError('JSON number out of range') from error
    if not math.isfinite(number):
        raise ValueError('JSON number out of range')
    if number == 0:
        return '-0' if math.copysign(1, number) < 0 else '0'
    sign, digits, exponent = Decimal(repr(number)).as_tuple()
    text = ''.join(map(str, digits))
    point = len(text) + exponent
    text = text.rstrip('0')
    prefix = '-' if sign else ''
    if 1e-6 <= abs(number) < 1e21:
        if point <= 0:
            return prefix + '0.' + '0' * -point + text
        if point >= len(text):
            return prefix + text + '0' * (point - len(text))
        return prefix + text[:point] + '.' + text[point:]
    power = point - 1
    mantissa = text[0] + ('.' + text[1:] if len(text) > 1 else '')
    return f"{prefix}{mantissa}e{'-' if power < 0 else '+'}{abs(power):0{1 if power < 0 else 2}d}"


def go_json(value):
    if value is None:
        return 'null'
    if value is True:
        return 'true'
    if value is False:
        return 'false'
    if isinstance(value, str):
        return go_string(value)
    if isinstance(value, (int, float)):
        return go_number(value)
    if isinstance(value, list):
        return '[' + ','.join(map(go_json, value)) + ']'
    if isinstance(value, dict):
        items = sorted(value.items(), key=lambda item: item[0].encode('utf-8'))
        return '{' + ','.join(go_string(key) + ':' + go_json(item) for key, item in items) + '}'
    raise ValueError('invalid JSON value')


def recipe_sha256(value):
    if not isinstance(value, dict):
        raise ValueError('invalid recipe')
    return hashlib.sha256(go_json(value).encode('utf-8')).hexdigest()
