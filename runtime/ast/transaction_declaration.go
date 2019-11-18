package ast

import "github.com/dapperlabs/flow-go/language/runtime/common"

type TransactionDeclaration struct {
	PreConditions  []*Condition
	PostConditions []*Condition
	Prepare        *SpecialFunctionDeclaration
	Execute        *Block
}

func (e *TransactionDeclaration) Accept(visitor Visitor) Repr {
	return visitor.VisitTransactionDeclaration(e)
}

func (*TransactionDeclaration) isDeclaration() {}
func (*TransactionDeclaration) isStatement()   {}

func (e *TransactionDeclaration) DeclarationName() string {
	return ""
}

func (e *TransactionDeclaration) DeclarationKind() common.DeclarationKind {
	return common.DeclarationKindTransaction
}
