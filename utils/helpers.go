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

func IsDayBefore(day1, day2 string) bool {
	dayMap := map[string]int{
		"Sun": 0,
		"Mon": 1,
		"Tue": 2,
		"Wed": 3,
		"Thu": 4,
		"Fri": 5,
		"Sat": 6}

	return dayMap[day1] < dayMap[day2]
}
