package calendar

import (
	"testing"
	"time"
)

func TestGridOctober2026(t *testing.T) {
	// Le 1er octobre 2026 est un jeudi.
	g := Grid(2026, time.October, true)
	if c := g[0][3]; c.Day != 1 || !c.InMonth {
		t.Errorf("le 1er devrait être un jeudi (colonne 3) : %+v", c)
	}
	if c := g[0][0]; c.Day != 28 || c.Month != time.September || c.InMonth {
		t.Errorf("début de grille : %+v", c)
	}
	g2 := Grid(2026, time.October, false)
	if c := g2[0][4]; c.Day != 1 {
		t.Errorf("semaine du dimanche : %+v", c)
	}
	n := 0
	for _, w := range g {
		for _, c := range w {
			if c.InMonth {
				n++
			}
		}
	}
	if n != 31 {
		t.Errorf("%d jours dans le mois", n)
	}
}

func TestAddAndNames(t *testing.T) {
	if y, m := Add(2026, time.December, 1); y != 2027 || m != time.January {
		t.Error(y, m)
	}
	if y, m := Add(2026, time.January, -1); y != 2025 || m != time.December {
		t.Error(y, m)
	}
	if MonthName(time.October, true) != "octobre" || DayName(time.Friday, false) != "Friday" {
		t.Error("noms")
	}
	if i := Initials(true, true); i[0] != "l" || i[6] != "d" {
		t.Error(i)
	}
}
