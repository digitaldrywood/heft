package withtests // want "package withtests has 3 test functions, 50% over the limit of 2 \\(target: 2 or fewer, 1 to remove\\)"

import "testing"

func TestA(t *testing.T)      {}
func TestB(t *testing.T)      {}
func Testlower(t *testing.T)  {}
func BenchmarkC(b *testing.B) {}
