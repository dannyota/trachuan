package run

import "testing"

func TestFetchReads_AttachmentCounts(t *testing.T) {
	var f FetchReads
	f.addFiles(`read=control outcome=changed files={"etag_md5_baseline":2,"control_file_changed":1,"failure_timeout":1}`)
	f.addFiles(`read=null_date files={"etag_md5_baseline":1,"etag_differs":3}`)
	if f.Attachments["etag_md5_baseline"] != 3 || f.Attachments["control_file_changed"] != 1 || f.Attachments["failure_timeout"] != 1 || f.Attachments["etag_differs"] != 3 {
		t.Fatal(f)
	}
}
