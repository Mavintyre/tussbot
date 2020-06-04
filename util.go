package main

// Clamp an integer between two values
func Clamp(num int, min int, max int) int {
	if num > max {
		return max
	}
	if num < min {
		return min
	}
	return num
}

// StrClamp returns a string clamped to max length
func StrClamp(str string, max int) string {
	length := len(str)
	if length > max {
		return str[:max]
	}
	return str
}
