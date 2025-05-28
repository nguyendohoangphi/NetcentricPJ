package main

type Tower struct {
	Name string
	HP   int
	ATK  int
	DEF  int
	CRIT float64
	EXP  int
}

func NewTower(name string, level int) Tower {
	var base Tower
	switch name {
	case "King Tower":
		base = Tower{Name: name, HP: 2000, ATK: 500, DEF: 300, CRIT: 0.1, EXP: 200}
	case "Guard Tower":
		base = Tower{Name: name, HP: 1000, ATK: 300, DEF: 100, CRIT: 0.05, EXP: 100}
	default:
		base = Tower{Name: name, HP: 500, ATK: 100, DEF: 50, CRIT: 0.0, EXP: 50}
	}

	scale := 1.0 + 0.1*float64(level-1)
	base.HP = int(float64(base.HP) * scale)
	base.ATK = int(float64(base.ATK) * scale)
	base.DEF = int(float64(base.DEF) * scale)
	return base
}
