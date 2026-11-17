from datetime import date
from pathlib import Path
import re


def decode_reviews(data):
    import yaml

    class ReviewLoader(yaml.SafeLoader):
        def construct_mapping(self, node, deep=False):
            result = {}
            for key_node, value_node in node.value:
                key = self.construct_object(key_node, deep=deep)
                if not isinstance(key, str) or key in result:
                    raise ValueError('invalid or duplicate heading review field')
                if key != 'projection' and isinstance(value_node, yaml.ScalarNode) and value_node.tag != 'tag:yaml.org,2002:null':
                    result[key] = self.construct_scalar(value_node)
                else:
                    result[key] = self.construct_object(value_node, deep=deep)
            return result

    try:
        return yaml.load(data, Loader=ReviewLoader)
    except yaml.YAMLError as error:
        raise ValueError('invalid heading review YAML') from error


def validate_heading_review(entry):
    fields = {'projection', 'scan_sha256', 'exclusions_sha256', 'reviewer', 'date'}
    if not isinstance(entry, dict) or set(entry) != fields:
        raise ValueError('invalid heading review fields')
    if type(entry['projection']) is not int or entry['projection'] != 1:
        raise ValueError('invalid heading review projection')
    for field in ('scan_sha256', 'exclusions_sha256'):
        if not isinstance(entry[field], str) or re.fullmatch(r'[0-9a-f]{64}', entry[field]) is None:
            raise ValueError('invalid heading review hash')
    if not isinstance(entry['reviewer'], str) or re.fullmatch(r'[a-z][a-z0-9_-]{0,63}', entry['reviewer']) is None:
        raise ValueError('heading reviewer must be a role')
    value = entry['date']
    if not isinstance(value, str) or re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}', value) is None:
        raise ValueError('invalid heading review date')
    date.fromisoformat(value)
    return entry


def heading_review(rel, path=None):
    if path is None:
        path = Path(__file__).resolve().parents[2] / 'sources/vn-heading-reviews.yaml'
    data = Path(path).read_text(encoding='utf-8')
    if not data.strip():
        return None
    entries = decode_reviews(data)
    if not isinstance(entries, list):
        raise ValueError('heading reviews must be a sequence')
    seen = set()
    selected = None
    for entry in entries:
        if not isinstance(entry, dict) or set(entry) != {'release', 'projection', 'scan_sha256', 'exclusions_sha256', 'reviewer', 'date'}:
            raise ValueError('invalid heading review entry fields')
        release = entry['release']
        if not isinstance(release, str) or re.fullmatch(r'[0-9]{8}(?:_[0-9]{1,2})?', release) is None or release in seen:
            raise ValueError('invalid or duplicate heading review release')
        seen.add(release)
        review = {key: value for key, value in entry.items() if key != 'release'}
        validate_heading_review(review)
        if release == rel:
            selected = review
    return selected
