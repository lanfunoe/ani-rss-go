package utils

// Deref 取指针指向的值，nil 时返回零值
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr 取指针指向的值，nil 时返回 def
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
