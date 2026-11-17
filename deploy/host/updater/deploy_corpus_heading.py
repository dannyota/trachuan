from datetime import date
import re


def validate_evidence(value):
    if not isinstance(value, dict) or set(value) != {'projection', 'scan_sha256', 'exclusions_sha256'}:
        raise ValueError('invalid heading evidence fields')
    if type(value['projection']) is not int or value['projection'] != 1:
        raise ValueError('invalid heading projection')
    for field in ('scan_sha256', 'exclusions_sha256'):
        if not isinstance(value[field], str) or re.fullmatch('[0-9a-f]{64}', value[field]) is None:
            raise ValueError('invalid heading evidence hash')


def validate_headings(manifest):
    if 'heading_evidence' in manifest:
        validate_evidence(manifest['heading_evidence'])
    if 'heading_review' in manifest:
        review = manifest['heading_review']
        if not isinstance(review, dict) or set(review) != {'projection', 'scan_sha256', 'exclusions_sha256', 'reviewer', 'date'}:
            raise ValueError('invalid heading review fields')
        validate_evidence({key: review[key] for key in ('projection', 'scan_sha256', 'exclusions_sha256')})
        if not isinstance(review['reviewer'], str) or re.fullmatch('[a-z][a-z0-9_-]{0,63}', review['reviewer']) is None:
            raise ValueError('invalid heading reviewer')
        if not isinstance(review['date'], str) or re.fullmatch('[0-9]{4}-[0-9]{2}-[0-9]{2}', review['date']) is None:
            raise ValueError('invalid heading review date')
        date.fromisoformat(review['date'])


def headings_approved(manifest):
    try:
        validate_headings(manifest)
    except ValueError:
        return False
    review, evidence = manifest.get('heading_review'), manifest.get('heading_evidence')
    return (isinstance(review, dict) and isinstance(evidence, dict)
            and all(type(review.get(key)) is type(evidence.get(key)) and review.get(key) == evidence.get(key)
                    for key in ('projection', 'scan_sha256', 'exclusions_sha256')))
