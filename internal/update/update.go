// Package update contient la logique des mises à jour de Lumen : lecture de la
// dernière « release » GitHub, comparaison des numéros de version et choix du
// bon fichier .msi. Il ne dépend pas de Windows et est testé partout.
package update

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Asset est un fichier joint à une release (ici, un installateur .msi).
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest vaut « sha256:<empreinte> » quand GitHub la fournit.
	Digest string `json:"digest"`
}

// Release est la réponse de l'API GitHub « releases/latest ».
type Release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	PageURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// LatestURL donne l'adresse de l'API pour la dernière release d'un dépôt « propriétaire/nom ».
func LatestURL(repo string) string {
	return "https://api.github.com/repos/" + repo + "/releases/latest"
}

// PageURL donne la page des releases d'un dépôt.
func PageURL(repo string) string {
	return "https://github.com/" + repo + "/releases"
}

// ParseRelease lit la réponse JSON de l'API GitHub.
func ParseRelease(data []byte) (Release, error) {
	var r Release
	err := json.Unmarshal(data, &r)
	return r, err
}

// parse transforme « v1.0.12 » (ou « 1.0.12-beta ») en [1 0 12].
func parse(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimLeft(v, "vV")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Valid dit si v ressemble à un numéro de version (« dev » n'en est pas un).
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

// Compare renvoie -1, 0 ou 1 selon que a est plus ancienne, égale ou plus récente que b.
// Un numéro illisible compte pour 0.
func Compare(a, b string) int {
	pa, _ := parse(a)
	pb, _ := parse(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Newer dit si latest est strictement plus récente que current.
func Newer(latest, current string) bool {
	return Valid(latest) && Compare(latest, current) > 0
}

// ArchName donne le nom d'architecture utilisé dans les noms des fichiers .msi.
func ArchName(goarch string) string {
	switch goarch {
	case "386":
		return "x86"
	case "arm64":
		return "arm64"
	default:
		return "x64"
	}
}

// AssetFor cherche l'installateur « Lumen-<arch>.msi » de la release.
func (r Release) AssetFor(arch string) (Asset, bool) {
	want := "lumen-" + arch + ".msi"
	for _, a := range r.Assets {
		if strings.ToLower(a.Name) == want {
			return a, true
		}
	}
	return Asset{}, false
}
