package cyberpunk

import (
	"encoding/json"
	"testing"
)

func TestParseSheetRoundtrip(t *testing.T) {
	raw := []byte(`{"body":6,"will":6,"emp":7,"ref":6,"skills":{"handgun":4},"cyberware":[{"name":"Neural Link","hl":2},{"name":"Cybereye","hl":3}],"sp":11,"weapons":[{"name":"Medium Pistol","skill":"handgun","damage":"2d6"}]}`)
	s, err := ParseSheet(raw)
	if err != nil {
		t.Fatalf("ParseSheet: %v", err)
	}
	if s.BODY != 6 || s.WILL != 6 || s.EMP != 7 || s.REF != 6 {
		t.Fatalf("STATS не разобрались: %+v", s)
	}
	if s.Skill("handgun") != 4 {
		t.Fatalf("skill handgun: %d", s.Skill("handgun"))
	}
	if s.TotalHL() != 5 {
		t.Fatalf("TotalHL: %d, want 5", s.TotalHL())
	}
	if s.Humanity() != 7*10-5 {
		t.Fatalf("Humanity: %d", s.Humanity())
	}
	if s.CurrentEMP() != 6 {
		t.Fatalf("CurrentEMP: %d (want floor(65/10)=6)", s.CurrentEMP())
	}
	if s.MaxHP() != 40 {
		t.Fatalf("MaxHP: %d (want (6+6)/2*5+10=40)", s.MaxHP())
	}
	if s.SP != 11 || len(s.Weapons) != 1 {
		t.Fatalf("SP/weapons: %+v", s)
	}
}

func TestParseSheetEmpty(t *testing.T) {
	s, err := ParseSheet(nil)
	if err != nil {
		t.Fatalf("пустой вход должен давать нулевой лист без ошибки: %v", err)
	}
	if s.Humanity() != 0 || s.CurrentEMP() != 0 || s.MaxHP() != 10 {
		t.Fatalf("нулевой лист неожиданный: HP=%d, Hum=%d, EMP=%d", s.MaxHP(), s.Humanity(), s.CurrentEMP())
	}
}

func TestMaxHPFormula(t *testing.T) {
	// RED: ((BODY+WILL)/2, округл. вверх) × 5 + 10.
	cases := []struct {
		body, will, want int
	}{
		{5, 5, 35},   // (10/2)*5+10 = 35
		{6, 7, 45},   // (13→7)*5+10 = 45
		{8, 8, 50},   // (16/2)*5+10 = 50
		{4, 5, 35},   // (9→5)*5+10 = 35
	}
	for _, c := range cases {
		got := MaxHP(c.body, c.will)
		if got != c.want {
			t.Errorf("MaxHP(%d,%d)=%d, want %d", c.body, c.will, got, c.want)
		}
	}
}

func TestSeriouslyMortallyWounded(t *testing.T) {
	if !SeriouslyWounded(20, 40) {
		t.Errorf("HP=20/40 обязан быть Seriously Wounded")
	}
	if SeriouslyWounded(21, 40) {
		t.Errorf("HP=21/40 не Seriously Wounded (порог — ½)")
	}
	if !MortallyWounded(-1) || MortallyWounded(0) {
		t.Errorf("MortallyWounded: <0 обязан, 0 — нет")
	}
}

func TestSheetJSONShape(t *testing.T) {
	// Регресс: поля листа сериализуются под ключами, ожидаемыми case.json.
	s := Sheet{BODY: 6, WILL: 6, EMP: 7, SP: 11}
	b, _ := json.Marshal(s)
	if want := `"body":6`; !contains(string(b), want) {
		t.Errorf("нет ключа %q в JSON: %s", want, b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
