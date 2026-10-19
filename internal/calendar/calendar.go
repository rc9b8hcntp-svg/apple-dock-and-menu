// Package calendar calcule la grille d'un mois (six semaines de sept jours),
// pour le calendrier qui s'ouvre au clic sur l'horloge. Il ne dépend pas de
// Windows et est testé partout.
package calendar

import "time"

// Cell est un jour de la grille.
type Cell struct {
	Year    int
	Month   time.Month
	Day     int
	InMonth bool // le jour appartient bien au mois affiché
}

// Grid renvoie les six semaines qui couvrent le mois ; mondayFirst choisit la
// semaine qui commence le lundi (sinon le dimanche).
func Grid(year int, month time.Month, mondayFirst bool) [6][7]Cell {
	first := time.Date(year, month, 1, 12, 0, 0, 0, time.UTC)
	offset := int(first.Weekday()) // dimanche = 0
	if mondayFirst {
		offset = (offset + 6) % 7
	}
	start := first.AddDate(0, 0, -offset)
	var g [6][7]Cell
	for w := 0; w < 6; w++ {
		for d := 0; d < 7; d++ {
			t := start.AddDate(0, 0, w*7+d)
			g[w][d] = Cell{Year: t.Year(), Month: t.Month(), Day: t.Day(), InMonth: t.Month() == month}
		}
	}
	return g
}

// Add décale un mois de delta mois (négatif pour reculer).
func Add(year int, month time.Month, delta int) (int, time.Month) {
	t := time.Date(year, month+time.Month(delta), 1, 12, 0, 0, 0, time.UTC)
	return t.Year(), t.Month()
}

var monthsFR = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
var monthsEN = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
var daysFR = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
var daysEN = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// MonthName donne le nom du mois.
func MonthName(m time.Month, fr bool) string {
	if fr {
		return monthsFR[m-1]
	}
	return monthsEN[m-1]
}

// DayName donne le nom du jour de la semaine.
func DayName(d time.Weekday, fr bool) string {
	if fr {
		return daysFR[d]
	}
	return daysEN[d]
}

// Initials donne les lettres des colonnes, dans l'ordre de la grille.
func Initials(mondayFirst, fr bool) [7]string {
	names := daysEN
	if fr {
		names = daysFR
	}
	var out [7]string
	for i := 0; i < 7; i++ {
		wd := i
		if mondayFirst {
			wd = (i + 1) % 7
		}
		out[i] = string([]rune(names[wd])[0:1])
		if !fr {
			out[i] = names[wd][:1]
		}
	}
	return out
}
