def cohort_counts(report):
    cohorts, rows = report.get('cohorts'), report.get('cases')
    if not isinstance(cohorts, dict) or not isinstance(rows, list):
        raise ValueError('missing report cohorts or case records')
    identities = {name: set() for name in cohorts}
    for row in rows:
        if not isinstance(row, dict) or not isinstance(row.get('case_id'), str) or not row['case_id'] or row.get('cohort') not in identities:
            raise ValueError('invalid case identity or cohort')
        identities[row['cohort']].add(row['case_id'])
    if 'old_product' in identities:
        rejections = report.get('rejections')
        if not isinstance(rejections, list):
            raise ValueError('missing rejection records')
        for row in rejections:
            if not isinstance(row, dict) or not isinstance(row.get('case_id'), str) or not row['case_id']:
                raise ValueError('invalid rejection identity')
            identities['old_product'].add(row['case_id'])
    counts = {name: len(ids) for name, ids in identities.items()}
    for name, count in counts.items():
        cohort = cohorts[name]
        if not isinstance(cohort, dict) or type(cohort.get('case_count')) is not int or cohort['case_count'] != count:
            raise ValueError('cohort case_count differs from case records: ' + name)
    return counts
