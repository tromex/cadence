// +build js,wasm

package main

import (
	"syscall/js"
)

func main() {
	done := make(chan struct{}, 0)
	js.Global().Set("__CADENCE_PARSE__", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		code := args[0].String()
		return parse(code)
	}))
	<-done
}
