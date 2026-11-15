import json


def release_floor(value):
    if not isinstance(value, dict):
        raise ValueError('invalid release floor policy')
    if 'release_floor' not in value:
        return 0
    floor = value['release_floor']
    if type(floor) is not int or floor < 1:
        raise ValueError('invalid release floor')
    return floor


def bundle_floor(files):
    return release_floor(json.loads(files['deploy-policy.json']))


def check_floor(sequence, floor):
    if sequence < floor:
        raise ValueError('image sequence ' + str(sequence) + ' is below release floor ' + str(floor))
