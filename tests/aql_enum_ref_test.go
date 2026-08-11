package tests

import (
	"strings"
	"testing"
)

const enumRefSchema = `
enum TransactionActorEntity { ApiKey, User }
type Transaction {
  required id: uuid;
  required sender_entity: TransactionActorEntity;
  required reciever_entity: TransactionActorEntity;
}
`

// An enum member can be referenced by qualified name (EnumName.Value) in an
// expression; since enums are stored as TEXT it lowers to a SQL string literal.
func TestEnumMemberReferenceCompiles(t *testing.T) {
	c := compileAQL(t, enumRefSchema,
		`select Transaction { id } filter .sender_entity = TransactionActorEntity.ApiKey;`)
	if !strings.Contains(c.SQL, "= 'ApiKey'") {
		t.Errorf("enum member reference should lower to 'ApiKey', got:\n%s", c.SQL)
	}
}

// Enum member references work inside grouped boolean filters too.
func TestEnumMemberReferenceInBooleanFilter(t *testing.T) {
	c := compileAQL(t, enumRefSchema,
		`select Transaction { id } filter (.sender_entity = TransactionActorEntity.ApiKey) or (.reciever_entity = TransactionActorEntity.User);`)
	if !strings.Contains(c.SQL, "'ApiKey'") || !strings.Contains(c.SQL, "'User'") {
		t.Errorf("both enum member references should lower to string literals, got:\n%s", c.SQL)
	}
}

// A qualified reference to a non-existent enum value is a compile error.
func TestEnumMemberReferenceUnknownValue(t *testing.T) {
	err := compileErr(t, enumRefSchema,
		`select Transaction { id } filter .sender_entity = TransactionActorEntity.Nope;`)
	if err == nil || !strings.Contains(err.Error(), `has no value "Nope"`) {
		t.Fatalf("expected an unknown-enum-value error, got: %v", err)
	}
}
