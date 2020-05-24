package main

import (
	"encoding/json"

	"github.com/onflow/cadence/runtime/parser2"
)

func parse(code string) string {
	program, _ := parser2.ParseProgram(code)
	res, _ := json.Marshal(program)
	return string(res)
}
