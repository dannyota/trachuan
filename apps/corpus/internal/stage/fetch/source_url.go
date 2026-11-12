package fetch

import "danny.vn/trachuan/apps/corpus/internal/vbpl"

func vbplSourceURL(id string) string {
	if !vbpl.ValidDocumentID(id) {
		return ""
	}
	return "https://vbpl.vn/van-ban/chi-tiet/" + id
}
