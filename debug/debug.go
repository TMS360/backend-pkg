package debug

import (
	"runtime"

	"github.com/k0kubun/pp"
)

func Dump(v ...interface{}) {
	_, _ = pp.Println(v...)
}

func Dd(v ...interface{}) {
	_, _ = pp.Println(v...)
	runtime.Goexit()
}
