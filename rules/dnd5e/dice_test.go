package dnd5e

import "testing"

func TestParseDiceSimple(t *testing.T) {
	d, err := ParseDice("2d6")
	if err != nil {
		t.Fatal(err)
	}
	if d.N != 2 || d.Sides != 6 || d.Mod != 0 || d.AbilityMod != "" {
		t.Fatalf("2d6 разобралось как %+v", d)
	}
}

func TestParseDiceWithAbility(t *testing.T) {
	d, err := ParseDice("1d8+str")
	if err != nil {
		t.Fatal(err)
	}
	if d.N != 1 || d.Sides != 8 || d.AbilityMod != "str" {
		t.Fatalf("1d8+str: %+v", d)
	}
}

func TestParseDiceWithNumericMod(t *testing.T) {
	d, err := ParseDice("1d6+2")
	if err != nil {
		t.Fatal(err)
	}
	if d.Mod != 2 {
		t.Fatalf("mod: %+v", d)
	}
}

func TestParseDiceInvalid(t *testing.T) {
	if _, err := ParseDice("nope"); err == nil {
		t.Fatal("невалидная строка не должна разобраться")
	}
}
