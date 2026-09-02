package dnd5e

import "testing"

func TestParseSheetReadsAbilities(t *testing.T) {
	raw := []byte(`{"str":14,"dex":16,"con":13,"int":11,"wis":12,"cha":10,"prof":2,
                    "max_hp":12,"ac":14,"speed":30,
                    "skills":["stealth","perception"],"saves":["dex"]}`)
	s, err := ParseSheet(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dex != 16 {
		t.Fatalf("dex: %d", s.Dex)
	}
	if s.Mod("dex") != 3 {
		t.Fatalf("mod dex: %d", s.Mod("dex"))
	}
	if s.Mod("cha") != 0 {
		t.Fatalf("mod cha 10: %d", s.Mod("cha"))
	}
	if !s.Proficient("stealth") {
		t.Fatal("stealth должен быть prof")
	}
	if s.Proficient("athletics") {
		t.Fatal("athletics НЕ должен быть prof")
	}
	if !s.SaveProficient("dex") {
		t.Fatal("dex-save должен быть prof")
	}
}

func TestParseSheetEmptyDoesNotPanic(t *testing.T) {
	_, err := ParseSheet(nil)
	if err != nil {
		t.Fatal(err)
	}
}
