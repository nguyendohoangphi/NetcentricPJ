package main

type Troop struct {
	Name    string
	HP      int
	ATK     int
	DEF     int
	MANA    int
	EXP     int
	Special string
}

func NewTroop(name string, level int) Troop {
	var base Troop
	switch name {
	case "Pawn":
		base = Troop{Name: name, HP: 50, ATK: 150, DEF: 100, MANA: 3, EXP: 5}
	case "Bishop":
		base = Troop{Name: name, HP: 100, ATK: 200, DEF: 150, MANA: 4, EXP: 10}
	case "Rook":
		base = Troop{Name: name, HP: 250, ATK: 200, DEF: 200, MANA: 5, EXP: 25}
	case "Knight":
		base = Troop{Name: name, HP: 200, ATK: 300, DEF: 150, MANA: 5, EXP: 25}
	case "Prince":
		base = Troop{Name: name, HP: 500, ATK: 400, DEF: 300, MANA: 6, EXP: 50}
	case "Queen":
		base = Troop{Name: name, HP: 0, ATK: 0, DEF: 0, MANA: 5, EXP: 30, Special: "heal"}
	default:
		base = Troop{Name: name, HP: 100, ATK: 100, DEF: 100, MANA: 2, EXP: 5}
	}

	scale := 1.0 + 0.1*float64(level-1)
	base.HP = int(float64(base.HP) * scale)
	base.ATK = int(float64(base.ATK) * scale)
	base.DEF = int(float64(base.DEF) * scale)
	return base
}
