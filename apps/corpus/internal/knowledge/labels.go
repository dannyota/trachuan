package knowledge

func rawLabel(kind string) string {
	switch kind {
	case "repeals":
		return "bãi bỏ"
	case "amends_supplements":
		return "sửa đổi, bổ sung"
	case "replaces":
		return "thay thế"
	case "suspends":
		return "ngưng hiệu lực"
	case "implements":
		return "quy định chi tiết, hướng dẫn thi hành"
	case "guided_by":
		return "được hướng dẫn bởi"
	case "legal_basis":
		return "căn cứ"
	case "cites":
		return "dẫn chiếu"
	case "references":
		return "liên quan"
	}
	return ""
}
