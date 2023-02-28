package utils

func IsContains[T comparable](elements []T, findThis T) bool {
	found := false
	for _, element := range elements {
		if findThis == element {
			found = true
		}
	}
	return found
}
