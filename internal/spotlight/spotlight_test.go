package spotlight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEval(t *testing.T) {
	ok := map[string]float64{
		"2+2": 4, "12*3,5": 42, "(1+2)*3": 9, "2^10": 1024, "10/4": 2.5, "-3+5": 2,
		"2 x 3": 6, "7 ÷ 2": 3.5, "1.5+1.5": 3, "2*-3": -6, "100-1=": 99,
	}
	for in, want := range ok {
		got, good := Eval(in)
		if !good || got != want {
			t.Errorf("Eval(%q) = %v, %v ; attendu %v", in, got, good, want)
		}
	}
	for _, in := range []string{"", "42", "abc", "2+", "(2+3", "1/0", "word 2+2", "2++", "photos", "x"} {
		if v, good := Eval(in); good {
			t.Errorf("Eval(%q) devrait échouer, a donné %v", in, v)
		}
	}
}

func TestFormat(t *testing.T) {
	if Format(2.5, true) != "2,5" || Format(42, false) != "42" || Format(0.1+0.2, false) != "0.3" {
		t.Error("format")
	}
}

func TestSearchOrderAndLimits(t *testing.T) {
	es := []Entry{
		{Name: "Visual Studio Code", Kind: App},
		{Name: "Vision", Kind: App},
		{Name: "Avis client.docx", Kind: Document},
		{Name: "Wi-Fi", Kind: Setting},
		{Name: "Écran", Kind: Setting},
		{Name: "Photos", Kind: Folder},
	}
	r := Search(es, "vis")
	if len(r) != 3 || r[0].Name != "Vision" || r[1].Name != "Visual Studio Code" || r[2].Kind != Document {
		t.Fatalf("classement inattendu : %+v", r)
	}
	if got := Search(es, "ecran"); len(got) != 1 || got[0].Name != "Écran" {
		t.Errorf("accents : %+v", got)
	}
	if got := Search(es, "studio code"); len(got) != 1 {
		t.Errorf("plusieurs mots : %+v", got)
	}
	if Search(es, "  ") != nil {
		t.Error("recherche vide")
	}
	var many []Entry
	for i := 0; i < 20; i++ {
		many = append(many, Entry{Name: "app" + string(rune('a'+i)), Kind: App})
	}
	if n := len(Search(many, "app")); n != limits[App] {
		t.Errorf("limite : %d", n)
	}
}

func TestBest(t *testing.T) {
	rs := []Result{{Entry{Name: "a"}, 3}, {Entry{Name: "b"}, 1}, {Entry{Name: "c"}, 1}}
	if Best(rs) != 1 || Best(nil) != -1 {
		t.Error("Best")
	}
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("x"), 0o644)
	}
	mk("rapport.pdf")
	mk("photo.JPG")
	mk("notes/idee.txt")
	mk("notes/a/b/c/d/profond.txt")
	mk(".cache/secret.txt")
	mk("node_modules/x.js")
	mk("desktop.ini")
	got := map[string]Kind{}
	for _, e := range Walk([]string{root}, 3, 100) {
		got[e.Name] = e.Kind
	}
	for name, k := range map[string]Kind{"rapport.pdf": PDF, "photo.JPG": Image, "idee.txt": Document, "notes": Folder} {
		if gk, ok := got[name]; !ok || gk != k {
			t.Errorf("%s : %v %v", name, gk, ok)
		}
	}
	for _, name := range []string{"secret.txt", "x.js", "desktop.ini", "profond.txt"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s ne devrait pas être indexé", name)
		}
	}
	if n := len(Walk([]string{root}, 3, 2)); n != 2 {
		t.Errorf("maxEntries : %d", n)
	}
}
