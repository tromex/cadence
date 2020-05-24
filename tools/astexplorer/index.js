async function loadParser() {
    const go = new Go();
    const wasm = await fetch("./main.wasm")
    const result = await WebAssembly.instantiateStreaming(wasm, go.importObject)
    go.run(result.instance)
}

function parseCadence(code) {
    const result = global['__CADENCE_PARSE__'](code)
    return JSON.parse(result)
}

window.onload = async (event) => {
    await loadParser();

    const output = document.getElementById("output")

    document.getElementById("input")
        .addEventListener("input", event => {
            const result = parseCadence(event.target.value)
            output.innerText = JSON.stringify(result, null, 4)
        })
}
