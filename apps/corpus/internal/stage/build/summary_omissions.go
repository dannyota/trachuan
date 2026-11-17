package build

const summaryOmissionSQL = `EXISTS (
 SELECT 1 FROM coverage_gap g
 WHERE g.document_id=d.id AND g.document_key=d.key AND g.stage='build'
 AND ((g.kind='summary_field_capped' AND g.detail IN ('number','title','type','issuer'))
 OR (g.kind='summary_row_capped' AND g.detail=''))
)`
