import json
from pathlib import Path
import re
import unittest


OCI = Path(__file__).parents[1] / 'oci'
BUCKET = 'trachuan-frameworks-releases'


def statements():
    found = []
    for line in (OCI / 'iam.tf').read_text().splitlines():
        if line.strip().startswith('statements '):
            found.extend(json.loads(line.split('=', 1)[1]))
    return found


def bucket(label):
    source = (OCI / 'storage.tf').read_text()
    return re.search(r'resource "oci_objectstorage_bucket" "' + label + r'" \{\n(.*?)\n\}', source, re.S).group(1)


class OCIPolicyTest(unittest.TestCase):
    def test_host_reads_every_policy_secret_from_vault(self):
        policy = json.loads(Path(__file__).with_name('deploy-policy.json').read_bytes())
        reads = [s for s in statements() if s.startswith('Allow dynamic-group trachuan-host to read secret-bundles ')]
        self.assertEqual(len(reads), 1)
        self.assertLessEqual(set(policy['secrets']), set(re.findall(r"target\.secret\.name = '([^']+)'", reads[0])))

    def test_frameworks_bucket_has_law_bucket_settings(self):
        self.assertEqual(bucket('frameworks_releases'),
                         bucket('releases').replace('"trachuan-releases"', '"' + BUCKET + '"'))

    def test_only_upload_frameworks_verify_and_host_reach_frameworks_bucket(self):
        where = "in compartment trachuan where target.bucket.name = '" + BUCKET + "'"
        self.assertEqual([s for s in statements() if BUCKET in s], [
            'Allow group trachuan-release-upload to read buckets ' + where,
            "Allow group trachuan-release-upload to manage objects in compartment trachuan where all "
            "{target.bucket.name = '" + BUCKET + "', any {request.permission = 'OBJECT_CREATE', "
            "request.permission = 'OBJECT_OVERWRITE', request.permission = 'OBJECT_INSPECT', "
            "request.permission = 'OBJECT_READ'}}",
            'Allow group trachuan-frameworks-verify to read buckets ' + where,
            'Allow group trachuan-frameworks-verify to read objects ' + where,
            'Allow dynamic-group trachuan-host to read buckets ' + where,
            'Allow dynamic-group trachuan-host to read objects ' + where,
        ])
        self.assertEqual([s for s in statements() if 'trachuan-frameworks-verify' in s],
                         ['Allow group trachuan-frameworks-verify to read buckets ' + where,
                          'Allow group trachuan-frameworks-verify to read objects ' + where])
