package cases

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func decodeFile(t *testing.T, raw []byte) File {
	t.Helper()
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("разбор эталонного дела: %v", err)
	}
	return f
}

func mutate(t *testing.T, edit func(*File)) error {
	t.Helper()
	raw, err := os.ReadFile("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	f := decodeFile(t, raw)
	edit(&f)
	return validateFile(f)
}

func TestMinimalCaseIsValid(t *testing.T) {
	if _, err := Load("testdata/minimal.json"); err != nil {
		t.Fatalf("эталонное дело не проходит валидацию: %v", err)
	}
}

func TestFactWithoutMandatoryHolderIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Mandatory = false })
	if err == nil || !strings.Contains(err.Error(), "mandatory") {
		t.Fatalf("факт без mandatory-держателя принят: %v", err)
	}
}

func TestDanglingHolderReferenceIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].HolderID = "e_nobody" })
	if err == nil || !strings.Contains(err.Error(), "e_nobody") {
		t.Fatalf("держатель-призрак принят: %v", err)
	}
}

func TestAsymmetricAdjacencyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Locations[1].Adjacent = nil })
	if err == nil || !strings.Contains(err.Error(), "смежност") {
		t.Fatalf("односторонний проход принят: %v", err)
	}
}

func TestMissingFlavourKeyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { delete(f.Flavour, "clock.suspicion.filled") })
	if err == nil || !strings.Contains(err.Error(), "clock.suspicion.filled") {
		t.Fatalf("отсутствующий ключ флейвора принят: %v", err)
	}
}

func TestTruthSlotWithoutTokenIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Truth.Why = "нет_такого_токена" })
	if err == nil || !strings.Contains(err.Error(), "why") {
		t.Fatalf("правильный ответ, недостижимый ни одним токеном, принят: %v", err)
	}
}

func TestUnknownVerbInGateIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Gate.Verbs = []string{"hack"} })
	if err == nil || !strings.Contains(err.Error(), "hack") {
		t.Fatalf("несуществующий глагол в gate принят: %v", err)
	}
}

func TestValidatorReportsEveryViolationAtOnce(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Mandatory = false
		f.FactHolders[0].HolderID = "e_nobody"
	})
	if err == nil {
		t.Fatal("нарушения не обнаружены")
	}
	if !strings.Contains(err.Error(), "mandatory") || !strings.Contains(err.Error(), "e_nobody") {
		t.Errorf("сообщено не всё: %v", err)
	}
}
