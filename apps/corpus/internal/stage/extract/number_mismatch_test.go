package extract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSelection_TranscriptionNumber(t *testing.T) {
	cases := []struct {
		name, id, header, number, title, detail string
		consolidated                            bool
	}{
		{"186164", "1", "Số: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN ban hành quy chuẩn kỹ thuật quốc gia về an toàn bức xạ", "header 47/2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"uppercase colon", "1", "SỐ: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ", "header 47/2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"space before colon", "1", "Số :47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ", "header 47/2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"space before slash", "1", "Số: 47 /2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ", "header 47 /2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"space after slash", "1", "Số: 47/ 2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ", "header 47/ 2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"title longer number", "1", "Số: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ theo 147/2025/TT-BKHCN", "header 47/2025/TT-BKHCN metadata 50/2025/TT-BKHCN", false},
		{"title exact number", "1", "Số: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ theo 47/2025/TT-BKHCN", "", false},
		{"correction 1860", "1", "QUYẾT ĐỊNH Về việc đính chính Thông tư số 14/2015/TT-NHNN", "1860/QĐ-NHNN", "Quyết định số 1860/QĐ-NHNN điều chỉnh ngân hàng", "", false},
		{"circular 10", "1", "Số: 10-NH5 THÔNG TƯ SỐ 10-NH5 đã ký quyết định số 107/QĐ-NH5", "10/TT-NH5", "Thông tư số 10/TT-NH5 điều chỉnh ngân hàng", "", false},
		{"decree reference", "1", "THÔNG TƯ Nghị định số 196-HĐBT", "15-BTC/TT", "Thông tư số 15-BTC/TT điều chỉnh tài chính", "", false},
		{"mixed case without colon", "1", "Số 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn bức xạ", "", false},
		{"7402", "1", "Số: 08/1998 THÔNG TƯ HƯỚNG DẪN QUẢN LÝ NGÂN SÁCH NHÀ NƯỚC", "06/1998", "Thông tư số 06/1998 hướng dẫn quản lý ngân sách nhà nước", "", false},
		{"14151", "1", "Số: 04/2006 QUYẾT ĐỊNH BAN HÀNH QUY CHẾ QUẢN LÝ TÀI CHÍNH", "49/2007", "Quyết định số 49/2007 ban hành quy chế quản lý tài chính", "", false},
		{"8443", "1", "Số: 1 THÔNG TƯ HƯỚNG DẪN THỰC HIỆN QUY ĐỊNH", "02/TT-NH7", "Thông tư số 02/TT-NH7 nội dung khác", "", false},
		{"1200 manual", "1200", "Số: 15/SL Ngày 06 tháng 05 năm 1951 SẮC LỆNH SỐ 14/SL NGÀY 1 THÁNG 5 NĂM 1951", "15/SL", "Sắc lệnh số 15/SL", "holds Sắc lệnh 14/SL of 1951-05-01; VBPL record error; checked 2026-11-12", false},
		{"1200 automatic absent", "1", "Số: 15/SL Ngày 06 tháng 05 năm 1951 SẮC LỆNH SỐ 14/SL NGÀY 1 THÁNG 5 NĂM 1951", "15/SL", "Sắc lệnh số 15/SL", "", false},
		{"second candidate", "1", "Số: 15/SL SỐ 14/SL THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "15/SL", "Thông tư số 15/SL ban hành quy chuẩn an toàn bức xạ", "header 14/SL metadata 15/SL", false},
		{"title named", "1", "SỐ 14/SL THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ ĐO LƯỜNG", "15/SL", "Thông tư số 15/SL sửa đổi 14/SL", "", false},
		{"consolidated", "1", "Số: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ", "50/2025/TT-BKHCN", "Thông tư số 50/2025/TT-BKHCN an toàn", "", true},
		{"VBHN", "1", "Số: 47/2025/TT-BKHCN THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ", "50/VBHN", "Thông tư số 50/VBHN an toàn", "", false},
		{"9747 and 109687", "1", "Số: 14TC/TTTN HƯỚNG DẪN BỔ SUNG THÔNG TƯ LIÊN BỘ SỐ 12/TTLB", "14TC/TTTN", "Hướng dẫn bổ sung thông tư liên bộ số 12/TTLB", "", false},
		{"correction notice", "1", "ĐÍNH CHÍNH NGHỊ ĐỊNH SỐ 125/2020/NĐ-CP Số: 125/2020/NĐ-CP", "125/2020/NĐ-CP", "", "", false},
		{"marker", "1", "Căn cứ luật Số: 47/SL THÔNG TƯ QUY ĐỊNH VỀ THIẾT BỊ", "50/SL", "Thông tư số 50/SL an toàn", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := headingBundle(t, `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi<br>Nội dung"}}]}`, false)
			data := []byte("<p>" + tc.header + "</p><p>Điều 1. Nội dung</p>")
			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "files", hash), data, 0600); err != nil {
				t.Fatal(err)
			}
			files := fmt.Sprintf(`[{"name":"%s_content.html","sha256":"%s","size":%d}]`, tc.id, hash, len(data))
			detail, err := json.Marshal(map[string]any{"data": map[string]any{"id": tc.id, "docNum": tc.number, "title": tc.title, "isConsolidatedDocument": tc.consolidated}})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "files.json"), []byte(files), 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "detail.json"), detail, 0600); err != nil {
				t.Fatal(err)
			}
			got, nodes, err := choose(context.Background(), dir, "vn:vbpl:"+tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.detail != "" {
				if len(nodes) != 0 {
					t.Fatalf("mismatch retained nodes: %v", nodes)
				}
				got, nodes, err = materializeFrozen(context.Background(), dir, "vn:vbpl:"+tc.id, got)
				if err != nil {
					t.Fatal(err)
				}
				if got.source != "none" || got.gap != "text_number_mismatch" || got.reason != tc.detail || len(nodes) != 0 || selectionInputHash("fetch", got) == "fetch" {
					t.Fatalf("selection=%+v nodes=%v", got, nodes)
				}
			} else if got.source != "source_tree" {
				t.Fatalf("selection=%+v", got)
			}
		})
	}
}

func TestSubjectMismatch_Overlap(t *testing.T) {
	for _, tc := range []struct {
		name, header, title string
		want                bool
	}{
		{"below half", "THÔNG TƯ alpha beta gamma delta", "Thông tư số 5/TT alpha omega sigma theta", true},
		{"half", "THÔNG TƯ alpha beta gamma delta", "Thông tư số 5/TT alpha beta sigma theta", false},
		{"type followed by number", "SẮC LỆNH SỐ 14/SL NGÀY 1 THÁNG 5 NĂM 1951", "Sắc lệnh số 15/SL nội dung khác", false},
		{"type followed by owner", "NGHỊ ĐỊNH CỦA CHÍNH PHỦ", "Nghị định số 5/ND nội dung khác", false},
		{"type word boundary", "LUẬTpháp alpha beta gamma", "Luật số 5/TT nội dung khác", false},
		{"no significant title", "THÔNG TƯ alpha beta gamma", "Thông tư số 5/TT 123", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := subjectMismatch(tc.header, tc.title); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSelection_NoTranscriptionDoesNotReadDetail(t *testing.T) {
	for _, files := range []struct {
		name, content string
		present       bool
	}{
		{"no file list", "", false},
		{"empty file list", `[]`, true},
		{"other file", `[{"name":"1_content_origin.html"}]`, true},
	} {
		t.Run(files.name, func(t *testing.T) {
			dir := t.TempDir()
			tree := `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi<br>Nội dung"}}]}`
			if err := os.WriteFile(filepath.Join(dir, "tree.json"), []byte(tree), 0600); err != nil {
				t.Fatal(err)
			}
			if files.present {
				if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(files.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			selected, nodes, err := choose(context.Background(), dir, "vn:vbpl:1")
			if err != nil || selected.source != "source_tree" || len(nodes) != 1 {
				t.Fatalf("selection=%+v nodes=%v err=%v", selected, nodes, err)
			}
			mismatch, err := frozenMismatch(dir, "vn:vbpl:1")
			if err != nil || mismatch.gap != "" {
				t.Fatalf("mismatch=%+v err=%v", mismatch, err)
			}
		})
	}
}

func TestSelection_ManualMismatchWithoutBundle(t *testing.T) {
	dir := t.TempDir()
	selected, nodes, err := choose(context.Background(), dir, "vn:vbpl:1200")
	if err != nil || selected.gap != "text_number_mismatch" || selected.reason != manualMismatch1200Detail || len(nodes) != 0 {
		t.Fatalf("selection=%+v nodes=%v err=%v", selected, nodes, err)
	}
	mismatch, err := frozenMismatch(dir, "vn:vbpl:1200")
	if err != nil || mismatch.gap != selected.gap || mismatch.reason != selected.reason {
		t.Fatalf("mismatch=%+v err=%v", mismatch, err)
	}
}

func TestSelection_TreeFixtureWithoutMetadata(t *testing.T) {
	dir := t.TempDir()
	tree := `{"success":true,"data":[{"id":"1","level":"Article","title":"Điều 1"}]}`
	if err := os.WriteFile(filepath.Join(dir, "tree.json"), []byte(tree), 0600); err != nil {
		t.Fatal(err)
	}
	selected, _, err := choose(context.Background(), dir, "vn:vbpl:1")
	if err != nil || selected.source != "source_tree" {
		t.Fatalf("selection=%+v err=%v", selected, err)
	}
}
