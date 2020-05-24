
## AST Explorer

This is a wrapper for Cadence's new parser, compiled to WebAssembly.

```sh
GOARCH=wasm GOOS=js go build -o main.wasm .
cp $(go env GOROOT)/misc/wasm/wasm_exec.js .
```
