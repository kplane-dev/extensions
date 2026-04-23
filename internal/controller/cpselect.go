package controller

// CPMatches reports whether a nested CP named cpName should be targeted
// given the EnabledExtension's controlPlanes list.
// ["*"] matches everything; otherwise the name must appear in the list.
func CPMatches(controlPlanes []string, cpName string) bool {
	for _, t := range controlPlanes {
		if t == "*" || t == cpName {
			return true
		}
	}
	return false
}
