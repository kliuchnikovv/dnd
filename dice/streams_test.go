package dice

import "testing"

func TestStreamsAreIndependent(t *testing.T) {
	a := NewSource(7)
	b := NewSource(7)

	// В прогоне A из стрима resolve тянут три раза, в прогоне B — сначала
	// два раза из постороннего стрима, потом три из resolve.
	wantA := []int{a.Stream("resolve").D20(), a.Stream("resolve").D20(), a.Stream("resolve").D20()}

	b.Stream("false_lead").D20()
	b.Stream("flavour").D20()
	gotB := []int{b.Stream("resolve").D20(), b.Stream("resolve").D20(), b.Stream("resolve").D20()}

	for i := range wantA {
		if wantA[i] != gotB[i] {
			t.Fatalf("бросок %d разошёлся: %d против %d — стримы не независимы",
				i, wantA[i], gotB[i])
		}
	}
}

func TestSameSeedSameSequence(t *testing.T) {
	x := NewSource(42).Stream("resolve")
	y := NewSource(42).Stream("resolve")
	for i := 0; i < 50; i++ {
		if v, w := x.D20(), y.D20(); v != w {
			t.Fatalf("бросок %d: %d != %d — один seed дал разные последовательности", i, v, w)
		}
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	x := NewSource(1).Stream("resolve")
	y := NewSource(2).Stream("resolve")
	same := 0
	for i := 0; i < 50; i++ {
		if x.D20() == y.D20() {
			same++
		}
	}
	if same == 50 {
		t.Fatal("разные seed дали одинаковую последовательность")
	}
}

func TestD20StaysInRange(t *testing.T) {
	s := NewSource(3).Stream("resolve")
	for i := 0; i < 10000; i++ {
		if v := s.D20(); v < 1 || v > 20 {
			t.Fatalf("d20 вернул %d вне диапазона 1..20", v)
		}
	}
}

func TestFixedDiceReplaysValuesThenRepeatsLast(t *testing.T) {
	d := Fixed(20, 1)
	if got := d.D20(); got != 20 {
		t.Errorf("первый бросок: %d, ожидалось 20", got)
	}
	if got := d.D20(); got != 1 {
		t.Errorf("второй бросок: %d, ожидалось 1", got)
	}
	if got := d.D20(); got != 1 {
		t.Errorf("третий бросок: %d, ожидалось повторение последнего (1)", got)
	}
}
