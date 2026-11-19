//go:build integration

package evidence

import (
	"bytes"
	"strings"
	"testing"
)

func TestSummary_AgainstReleaseDatabase(t *testing.T) {
	f := newReleaseFixture(t)
	f.document(1, "vn:vbpl:2099001", "1/2099/TT-NHNN", "Một", "partial", "source_tree", "")
	f.document(2, "vn:vbpl:2099002", "2/2099/TT-NHNN", "Hai", "in_force", "pdf_text", "")
	f.exec(`UPDATE document SET eff_from='2099-01-02', eff_to='2099-12-31', validity_conflict=true, text_complete=true WHERE id=1`)
	f.gap(1, "build", "baseline_served", "vn:vbpl:2099002")
	f.provision(10, 1, nil, "dieu", "dieu-1", "Điều 1. Phạm vi điều chỉnh", 1)
	parent := int64(10)
	f.provision(11, 1, &parent, "khoan", "dieu-1/khoan-1", "", 1)
	f.provision(12, 1, nil, "dieu", "dieu-1~2", "Điều 1 lặp", 2)
	f.provision(20, 2, nil, "dieu", "dieu-5", "Điều 5", 1)
	f.exec(`INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES
 (1,'vn',NULL,1,'1','Chủ đề','','phapdien','active','n1',1,3,1),
 (2,'vn',1,2,'1.1','Đề mục','','phapdien','active','n1.n2',1,3,1),
 (3,'vn',2,3,'1.1.a','Nút','','phapdien','active','n1.n2.n3',1,3,1),
 (4,'vn',2,3,'1.1.r','Cũ','','phapdien','retired','n1.n2.n4',2,0,0)`)
	f.exec(`INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES
 (10,3,2,'phapdien_seed',false,1,'v1'),(10,4,2,'phapdien_seed',false,1,'v1')`)
	f.exec(`INSERT INTO provision_validity(id,provision_id,eff_from,eff_to,status_class,evidence,source_url,method,observed_at) VALUES
 (1,11,NULL,'2099-06-30','expired','e','https://vbpl.vn/x','text','2099-01-01T00:00:00Z')`)
	f.exec(`INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,source_provision_id,target_provision_id,evidence,method,artifact_sha256,ordinal,target_node_id) VALUES
 (1,2,1,'vn:vbpl:2099002','vn:vbpl:2099001','2/2099/TT-NHNN','1/2099/TT-NHNN','amends_supplements','x','x',20,10,'e','text_rule/1',$1,0,'dieu-1/khoan-7'),
 (2,2,NULL,'vn:vbpl:2099002',NULL,'2/2099/TT-NHNN','9/2098/TT','cites','x','x',NULL,NULL,'e','vbpl_diagram',$1,1,'node-9')`, hash64("a"))

	m := activeMembers("vn:vbpl:2099001", "vn:vbpl:2099002", "vn:vbpl:2099003")
	var buf bytes.Buffer
	if err := writeIdentity(f.ctx, &buf, sqlRelease{f.pool}, m, identityTestBindings(testRel)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"type":"document","key":"vn:vbpl:2099001","number_norm":"1/2099/TT-NHNN","status_class":"partial","eff_from":"2099-01-02","eff_to":"2099-12-31","is_current":true,"validity_conflict":true,"text_source":"source_tree","text_complete":true}`,
		`{"type":"document","key":"vn:vbpl:2099002","number_norm":"2/2099/TT-NHNN","status_class":"in_force","eff_from":null,"eff_to":null,"is_current":true,"validity_conflict":false,"text_source":"pdf_text","text_complete":false,"baseline":true}`,
		`{"type":"document","key":"vn:vbpl:2099003","absent":true}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1","parent":null,"ambiguous":true,"text_sha256":"` + textHash("Điều 1. Phạm vi điều chỉnh") + `","nodes":["1.1.a"],"validity":[]}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1/khoan-1","parent":"dieu-1","ambiguous":true,"text_sha256":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","nodes":["1.1.a"],"validity":[{"status_class":"expired","eff_from":null,"eff_to":"2099-06-30","method":"text"}]}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1~2","parent":null,"ambiguous":true,"text_sha256":"` + textHash("Điều 1 lặp") + `","nodes":[],"validity":[]}`,
		`{"type":"provision","key":"vn:vbpl:2099002","anchor":"dieu-5","parent":null,"text_sha256":"` + textHash("Điều 5") + `","nodes":[],"validity":[]}`,
		`{"type":"relation","kind":"amends_supplements","method":"text_rule/1","source":{"key":"vn:vbpl:2099002","number":"2/2099/TT-NHNN","in_release":true,"anchor":"dieu-5"},"target":{"key":"vn:vbpl:2099001","number":"1/2099/TT-NHNN","in_release":true,"anchor":"dieu-1","exact":false,"named":"dieu-1/khoan-7"},"count":1}`,
		`{"type":"relation","kind":"cites","method":"vbpl_diagram","source":{"key":"vn:vbpl:2099002","number":"2/2099/TT-NHNN","in_release":true,"anchor":null},"target":{"key":null,"number":"9/2098/TT","in_release":false,"anchor":null,"node":"node-9"},"count":1}`,
	}
	got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")[1:]
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("identity from the release database:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
