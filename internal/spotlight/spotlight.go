// Package spotlight contient la logique de la recherche façon Spotlight : le
// calcul d'une expression tapée (« 12*3,5 »), l'indexation des fichiers de
// l'utilisateur et le classement des résultats. Il ne dépend pas de Windows et
// est testé partout ; la fenêtre est dans spotlight_windows.go.
package spotlight

import (
	"io/fs"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"menuwin/internal/launchpad"
)

// ---------------------------------------------------------------------------
// Calcul
// ---------------------------------------------------------------------------

type parser struct {
	s   []rune
	pos int
	bad bool
	ops int // nombre d'opérateurs rencontrés
}

func (p *parser) skip() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) peek() rune {
	p.skip()
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *parser) expr() float64 {
	v := p.term()
	for {
		switch p.peek() {
		case '+':
			p.pos++
			p.ops++
			v += p.term()
		case '-', '−':
			p.pos++
			p.ops++
			v -= p.term()
		default:
			return v
		}
	}
}

func (p *parser) term() float64 {
	v := p.power()
	for {
		switch p.peek() {
		case '*', 'x', 'X', '×':
			p.pos++
			p.ops++
			v *= p.power()
		case '/', '÷', ':':
			p.pos++
			p.ops++
			d := p.power()
			if d == 0 {
				p.bad = true
				return 0
			}
			v /= d
		default:
			return v
		}
	}
}

func (p *parser) power() float64 {
	v := p.unary()
	if p.peek() == '^' {
		p.pos++
		p.ops++
		e := p.power()
		v = math.Pow(v, e)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			p.bad = true
		}
	}
	return v
}

func (p *parser) unary() float64 {
	switch p.peek() {
	case '-', '−':
		p.pos++
		return -p.unary()
	case '+':
		p.pos++
		return p.unary()
	}
	return p.primary()
}

func (p *parser) primary() float64 {
	c := p.peek()
	if c == '(' {
		p.pos++
		v := p.expr()
		if p.peek() != ')' {
			p.bad = true
			return 0
		}
		p.pos++
		return v
	}
	start := p.pos
	for p.pos < len(p.s) && (p.s[p.pos] >= '0' && p.s[p.pos] <= '9' || p.s[p.pos] == '.' || p.s[p.pos] == ',') {
		p.pos++
	}
	if start == p.pos {
		p.bad = true
		return 0
	}
	txt := strings.ReplaceAll(string(p.s[start:p.pos]), ",", ".")
	v, err := strconv.ParseFloat(txt, 64)
	if err != nil {
		p.bad = true
		return 0
	}
	return v
}

// Eval calcule une expression arithmétique (+ - * / x × ÷ ^ et parenthèses ; la
// virgule ou le point font la décimale). ok est faux si le texte n'est pas un
// calcul, ou s'il ne contient aucun opérateur (« 42 » n'est pas un calcul).
func Eval(text string) (float64, bool) {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "="))
	if text == "" {
		return 0, false
	}
	p := &parser{s: []rune(text)}
	v := p.expr()
	p.skip()
	if p.bad || p.pos != len(p.s) || p.ops == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// Format écrit un résultat sans zéros inutiles ; comma choisit la virgule décimale.
func Format(v float64, comma bool) string {
	v = math.Round(v*1e10) / 1e10
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if comma {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// ---------------------------------------------------------------------------
// Entrées et classement
// ---------------------------------------------------------------------------

// Kind est la catégorie d'un résultat ; l'ordre est celui des sections affichées.
type Kind int

const (
	App Kind = iota
	Setting
	Folder
	Document
	PDF
	Image
)

// Entry est une chose qu'on peut trouver : une application, un réglage, un fichier…
type Entry struct {
	Name   string
	Path   string // chemin, adresse (ms-settings:…) ou identifiant interne
	Detail string // dossier parent pour un fichier
	Kind   Kind
}

// Result est une entrée trouvée, avec son score (plus petit = meilleur).
type Result struct {
	Entry
	Score int
}

// limits : nombre maximal de résultats par catégorie.
var limits = map[Kind]int{App: 6, Setting: 3, Folder: 4, Document: 5, PDF: 3, Image: 3}

// score note la correspondance d'un nom avec la recherche : 0 identique, 1 commence
// par, 2 un mot commence par, 3 contient ; -1 si un des mots de la recherche manque.
func score(name, query string) int {
	f := launchpad.Fold(name)
	spaced := " " + strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return ' '
	}, f)
	worst := 0
	for _, tok := range strings.Fields(launchpad.Fold(query)) {
		s := -1
		switch {
		case f == tok:
			s = 0
		case strings.HasPrefix(f, tok):
			s = 1
		case strings.Contains(spaced, " "+tok):
			s = 2
		case strings.Contains(f, tok):
			s = 3
		}
		if s < 0 {
			return -1
		}
		if s > worst {
			worst = s
		}
	}
	return worst
}

// Search renvoie les entrées qui correspondent, rangées par catégorie (dans l'ordre
// de Kind), les meilleures d'abord, avec au plus quelques résultats par catégorie.
func Search(entries []Entry, query string) []Result {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	var all []Result
	for _, e := range entries {
		if s := score(e.Name, query); s >= 0 {
			all = append(all, Result{e, s})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Score != b.Score {
			return a.Score < b.Score
		}
		if len(a.Name) != len(b.Name) {
			return len(a.Name) < len(b.Name)
		}
		return launchpad.Fold(a.Name) < launchpad.Fold(b.Name)
	})
	var out []Result
	count := map[Kind]int{}
	for _, r := range all {
		if count[r.Kind] >= limits[r.Kind] {
			continue
		}
		count[r.Kind]++
		out = append(out, r)
	}
	return out
}

// Best donne l'indice du « meilleur résultat » : le meilleur score, et à score égal
// la catégorie la plus haute. -1 s'il n'y a rien.
func Best(rs []Result) int {
	best := -1
	for i, r := range rs {
		if best < 0 || r.Score < rs[best].Score {
			best = i
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Fichiers
// ---------------------------------------------------------------------------

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".webp": true, ".heic": true, ".svg": true, ".tif": true, ".tiff": true, ".ico": true}

var skipExt = map[string]bool{".ini": true, ".tmp": true, ".lnk": true, ".db": true, ".dat": true,
	".log": true, ".sys": true, ".dll": true, ".bak": true}

var skipDir = map[string]bool{"node_modules": true, "appdata": true, "$recycle.bin": true,
	"system volume information": true, "__pycache__": true}

// KindOfFile donne la catégorie d'un fichier d'après son extension.
func KindOfFile(name string) Kind {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case ext == ".pdf":
		return PDF
	case imageExt[ext]:
		return Image
	}
	return Document
}

// Walk indexe les fichiers et dossiers sous les dossiers donnés (jusqu'à maxDepth
// niveaux et maxEntries entrées) ; les éléments cachés et techniques sont ignorés.
func Walk(roots []string, maxDepth, maxEntries int) []Entry {
	var out []Entry
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if len(out) >= maxEntries {
				return fs.SkipAll
			}
			if p == root {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			depth := strings.Count(rel, string(filepath.Separator)) + 1
			if d.IsDir() {
				if skipDir[strings.ToLower(name)] || depth > maxDepth {
					return fs.SkipDir
				}
				out = append(out, Entry{Name: name, Path: p, Detail: filepath.Base(filepath.Dir(p)), Kind: Folder})
				return nil
			}
			if skipExt[strings.ToLower(filepath.Ext(name))] || depth > maxDepth+1 {
				return nil
			}
			out = append(out, Entry{Name: name, Path: p, Detail: filepath.Base(filepath.Dir(p)), Kind: KindOfFile(name)})
			return nil
		})
	}
	return out
}
