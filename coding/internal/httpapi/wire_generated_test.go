package httpapi

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestGeneratedWireContractIsCurrent(t *testing.T) {
	command := exec.Command("go", "run", "./internal/genwire", "-source", "wire_contract.go", "-output", "-")
	generated, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generate wire contract: %v\n%s", err, generated)
	}

	checkedIn, err := os.ReadFile("../../client/src/generated/wire.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("generated wire contract is stale; run go generate ./coding/internal/httpapi")
	}
}
