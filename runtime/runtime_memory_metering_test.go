/*
 * Cadence - The resource-oriented smart contract programming language
 *
 * Copyright 2019-2020 Dapper Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package runtime

import (
	"fmt"
	"github.com/onflow/cadence/encoding/json"
	"github.com/onflow/cadence/runtime/interpreter"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/runtime/tests/utils"
)

type testMemoryGauge struct {
	meter map[common.MemoryKind]uint64
}

func newTestMemoryGauge() *testMemoryGauge {
	return &testMemoryGauge{
		meter: make(map[common.MemoryKind]uint64),
	}
}

func (g *testMemoryGauge) MeterMemory(usage common.MemoryUsage) error {
	g.meter[usage.Kind] += usage.Amount
	return nil
}

func (g *testMemoryGauge) getMemory(kind common.MemoryKind) uint64 {
	return g.meter[kind]
}

func TestInterpreterAddressLocationMetering(t *testing.T) {

	t.Parallel()

	t.Run("add contract", func(t *testing.T) {
		t.Parallel()

		script := `
		pub struct S {}

		pub fun main() {
			let s = CompositeType("A.0000000000000001.S")
		}
        `
		meter := newTestMemoryGauge()
		var accountCode []byte
		runtimeInterface := &testRuntimeInterface{
			getSigningAccounts: func() ([]Address, error) {
				return []Address{{42}}, nil
			},
			storage: newTestLedger(nil, nil),
			meterMemory: func(usage common.MemoryUsage) error {
				return meter.MeterMemory(usage)
			},
			getAccountContractCode: func(_ Address, _ string) (code []byte, err error) {
				return accountCode, nil
			},
		}

		runtime := newTestInterpreterRuntime()

		_, err := runtime.ExecuteScript(
			Script{
				Source: []byte(script),
			},
			Context{
				Interface: runtimeInterface,
				Location:  utils.TestLocation,
			},
		)
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindAddressLocation))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindElaboration))
		assert.Equal(t, uint64(92), meter.getMemory(common.MemoryKindRawString))
	})
}

func TestInterpreterElaborationImportMetering(t *testing.T) {

	t.Parallel()

	contracts := [...][]byte{
		[]byte(`pub contract C0 {}`),
		[]byte(`pub contract C1 {}`),
		[]byte(`pub contract C2 {}`),
		[]byte(`pub contract C3 {}`),
	}

	importExpressions := [len(contracts)]string{}
	for i := range contracts {
		importExpressions[i] = fmt.Sprintf("import C%d from 0x1\n", i)
	}

	addressValue := cadence.BytesToAddress([]byte{byte(1)})

	for imports := range contracts {

		t.Run(fmt.Sprintf("import %d", imports), func(t *testing.T) {

			t.Parallel()

			script := "pub fun main() {}"
			for j := 0; j <= imports; j++ {
				script = importExpressions[j] + script
			}

			runtime := newTestInterpreterRuntime()

			meter := newTestMemoryGauge()

			accountCodes := map[common.LocationID][]byte{}

			runtimeInterface := &testRuntimeInterface{
				getCode: func(location Location) (bytes []byte, err error) {
					return accountCodes[location.ID()], nil
				},
				storage: newTestLedger(nil, nil),
				getSigningAccounts: func() ([]Address, error) {
					return []Address{Address(addressValue)}, nil
				},
				resolveLocation: singleIdentifierLocationResolver(t),
				updateAccountContractCode: func(address Address, name string, code []byte) error {
					location := common.AddressLocation{
						Address: address,
						Name:    name,
					}
					accountCodes[location.ID()] = code
					return nil
				},
				getAccountContractCode: func(address Address, name string) (code []byte, err error) {
					location := common.AddressLocation{
						Address: address,
						Name:    name,
					}
					code = accountCodes[location.ID()]
					return code, nil
				},
				meterMemory: func(usage common.MemoryUsage) error {
					return meter.MeterMemory(usage)
				},
				emitEvent: func(_ cadence.Event) error {
					return nil
				},
			}

			nextTransactionLocation := newTransactionLocationGenerator()

			for j := 0; j <= imports; j++ {
				err := runtime.ExecuteTransaction(
					Script{
						Source: utils.DeploymentTransaction(fmt.Sprintf("C%d", j), contracts[j]),
					},
					Context{
						Interface: runtimeInterface,
						Location:  nextTransactionLocation(),
					},
				)
				require.NoError(t, err)
				// one for each deployment transaction and one for each contract
				assert.Equal(t, uint64(2*j+2), meter.getMemory(common.MemoryKindElaboration))
			}

			_, err := runtime.ExecuteScript(
				Script{
					Source: []byte(script),
				},
				Context{
					Interface: runtimeInterface,
					Location:  nextTransactionLocation(),
				},
			)
			require.NoError(t, err)

			// in addition to the elaborations metered above, we also meter
			// one more for the script and one more for each contract imported
			assert.Equal(t, uint64(3*imports+4), meter.getMemory(common.MemoryKindElaboration))
		})
	}
}

func TestAuthAccountMetering(t *testing.T) {

	t.Parallel()

	t.Run("add keys", func(t *testing.T) {
		t.Parallel()

		meter := newTestMemoryGauge()
		var m runtime.MemStats
		var startMem uint64
		var lastMem uint64
		mRef := &m

		funcInvocHandler := interpreter.WithOnFunctionInvocationHandler(func(_ *interpreter.Interpreter, _ int) {
			meter.meter = make(map[common.MemoryKind]uint64)
			runtime.ReadMemStats(mRef)
			startMem = m.TotalAlloc
		})

		funcReturnHandler := interpreter.WithOnInvokedFunctionReturnHandler(func(_ *interpreter.Interpreter, _ int) {
			runtime.ReadMemStats(mRef)
			fmt.Println(m.TotalAlloc-startMem, " | diff:", m.TotalAlloc-lastMem)
			lastMem = m.TotalAlloc
			fmt.Println(meter.meter)
			fmt.Println()
		})

		rt := newTestInterpreterRuntime()

		script := []byte(`
            transaction {
                prepare(signer: AuthAccount) {
                    signer.addPublicKey("f847b84000fb479cb398ab7e31d6f048c12ec5b5b679052589280cacde421af823f93fe927dfc3d1e371b172f97ceeac1bc235f60654184c83f4ea70dd3b7785ffb3c73802038203e8".decodeHex())
                }
            }
        `)

		runtimeInterface := &testRuntimeInterface{
			getAccountContractNames: func(_ Address) ([]string, error) {
				return []string{"foo", "bar"}, nil
			},
			storage: newTestLedger(nil, nil),
			getSigningAccounts: func() ([]Address, error) {
				return []Address{{42}}, nil
			},
			createAccount: func(payer Address) (address Address, err error) {
				return Address{42}, nil
			},
			addAccountKey: func(address Address, publicKey *PublicKey, hashAlgo HashAlgorithm, weight int) (*AccountKey, error) {
				return &AccountKey{
					KeyIndex:  0,
					PublicKey: publicKey,
					HashAlgo:  hashAlgo,
					Weight:    weight,
					IsRevoked: false,
				}, nil
			},
			addEncodedAccountKey: func(address Address, publicKey []byte) error {
				return nil
			},

			getAccountKey: func(address Address, index int) (*AccountKey, error) {
				return nil, nil
			},
			removeAccountKey: func(address Address, index int) (*AccountKey, error) {
				return nil, nil
			},
			log: func(message string) {
			},
			emitEvent: func(event cadence.Event) error {
				return nil
			},
			meterMemory: func(_ common.MemoryUsage) error {
				return nil
			},
		}

		nextTransactionLocation := newTransactionLocationGenerator()

		err := rt.ExecuteTransaction(
			Script{
				Source: script,
			},
			Context{
				Interface: runtimeInterface,
				Location:  nextTransactionLocation(),
			},
			funcInvocHandler,
			funcReturnHandler,
			interpreter.WithMemoryGauge(meter),
		)

		require.NoError(t, err)
	})
}

func TestContractFunctionCall(t *testing.T) {
	interpreterRuntime := newTestInterpreterRuntime()

	contractsAddress := common.MustBytesToAddress([]byte{0x1})
	//senderAddress := common.MustBytesToAddress([]byte{0x2})
	//receiverAddress := common.MustBytesToAddress([]byte{0x3})

	accountCodes := map[common.LocationID][]byte{}

	var events []cadence.Event

	signerAccount := contractsAddress

	runtimeInterface := &testRuntimeInterface{
		getCode: func(location Location) (bytes []byte, err error) {
			return accountCodes[location.ID()], nil
		},
		storage: newTestLedger(nil, nil),
		getSigningAccounts: func() ([]Address, error) {
			return []Address{signerAccount}, nil
		},
		resolveLocation: singleIdentifierLocationResolver(t),
		getAccountContractCode: func(address Address, name string) (code []byte, err error) {
			location := common.AddressLocation{
				Address: address,
				Name:    name,
			}
			return accountCodes[location.ID()], nil
		},
		updateAccountContractCode: func(address Address, name string, code []byte) error {
			location := common.AddressLocation{
				Address: address,
				Name:    name,
			}
			accountCodes[location.ID()] = code
			return nil
		},
		emitEvent: func(event cadence.Event) error {
			events = append(events, event)
			return nil
		},
		meterMemory: func(_ common.MemoryUsage) error {
			return nil
		},
	}
	runtimeInterface.decodeArgument = func(b []byte, t cadence.Type) (value cadence.Value, err error) {
		return json.Decode(runtimeInterface, b)
	}

	nextTransactionLocation := newTransactionLocationGenerator()

	// Deploy  contract

	contractCode := `
			access(all) contract TestContract {
				access(all) fun createEmptyStruct(): Bar {
					return Bar()
				}

				pub struct Bar {
					init() {}
				}

				init() {}
			}
	`
	err := interpreterRuntime.ExecuteTransaction(
		Script{
			Source: utils.DeploymentTransaction(
				"TestContract",
				[]byte(contractCode),
			),
		},
		Context{
			Interface: runtimeInterface,
			Location:  nextTransactionLocation(),
		},
	)
	require.NoError(t, err)

	// Send transaction

	//signerAccount = contractsAddress

	script := `
			import TestContract from 0x1

            pub fun main() {
                var i = 0
                while i < 10 {
                    var a = TestContract.createEmptyStruct()
                    i = i + 1
				}
            }
        `

	meter := newTestMemoryGauge()

	var m runtime.MemStats
	var startMem uint64
	var lastMem uint64

	mRef := &m

	_, err = interpreterRuntime.ExecuteScript(
		Script{
			Source: []byte(script),
		},
		Context{
			Interface: runtimeInterface,
			Location:  nextTransactionLocation(),
		},
		interpreter.WithOnFunctionInvocationHandler(func(_ *interpreter.Interpreter, _ int) {
			//meter.meter = make(map[common.MemoryKind]uint64)
			runtime.ReadMemStats(mRef)
			startMem = m.TotalAlloc
			lastMem = startMem
		}),
		interpreter.WithOnInvokedFunctionReturnHandler(func(_ *interpreter.Interpreter, _ int) {
			runtime.ReadMemStats(mRef)
			fmt.Println(m.TotalAlloc-startMem, "diff:", m.TotalAlloc-lastMem)
			lastMem = m.TotalAlloc
			//fmt.Println(meter.meter)
		}),
		interpreter.WithTracingEnabled(false),
		interpreter.WithAtreeValueValidationEnabled(false),
		interpreter.WithAtreeStorageValidationEnabled(false),
		interpreter.WithMemoryGauge(meter),
	)
	require.NoError(t, err)

}
