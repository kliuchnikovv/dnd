package vignette

// Ruling — вердикт холодного судьи: что за действие, во что бьём, каким статом,
// какая сложность (DC уже разрешён по каноничной шкале), помогает/мешает ли
// обстановка (Adv), и — для improvise — пускать ли внесённый предмет. Судья
// правды сцены не видит; ядро исполняет вердикт честной костью.
type Ruling struct {
	Kind   string // look listen search moveon moveoff call interact recall improvise idle
	Target string
	Stat   string // edge | body
	DC     int
	Adv    int    // -1 помеха · 0 ровно · +1 преимущество (обстановка, не DC)
	Admit  string // improvise: grant | deny
	Item   string // improvise: короткое имя внесённого предмета
	Reason string // холодное обоснование (для лога)
}
