package shellvar

func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !isASCIIAlpha(r) && r != '_' {
				return false
			}
			continue
		}
		if !isASCIIAlpha(r) && !isASCIIDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

func isASCIIDigit(r rune) bool {
	return '0' <= r && r <= '9'
}
