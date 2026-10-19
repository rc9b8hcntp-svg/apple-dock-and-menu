//go:build windows

package main

// Mises à jour de Lumen (Réglages > Mises à jour).
//
// Le programme interroge l'API GitHub (dernière « release » du dépôt), compare
// son numéro à celui de la version installée, et, sur demande, télécharge
// l'installateur .msi qui correspond à ce PC puis l'ouvre (Windows Installer demande
// alors l'autorisation d'administrateur). L'installateur ferme Lumen, remplace
// les fichiers et le relance tout seul.
//
// Tout le réseau se fait en arrière-plan : les goroutines ne touchent qu'à
// `upd` (sous verrou). L'interface relit cet état toutes les 500 ms
// (updateTick, sur le fil de la barre) ; c'est aussi lui qui lance
// l'installateur, car ShellExecute doit partir du fil de l'interface.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"menuwin/internal/update"
)

// updateRepo : dépôt GitHub où sont publiées les versions (« propriétaire/nom »).
const updateRepo = "rc9b8hcntp-svg/apple-dock-and-menu"

// appVersion est le numéro de la version installée, fixé à la compilation :
//
//	go build -ldflags "-X main.appVersion=1.0.12"
//
// Une version compilée à la main (sans cette option) vaut « dev ».
var appVersion = "dev"

const updateEvery = 6 * time.Hour // recherche automatique : au démarrage puis toutes les 6 h

const (
	updIdle = iota
	updChecking
	updUpToDate
	updAvailable
	updDownloading
	updInstalling
	updError
)

const (
	errNone = iota // aucune release publiée
	errNet         // pas de connexion
	errRate        // trop de requêtes (limite de GitHub)
	errOther
	errDownload
	errInstall
)

var upd struct {
	mu        sync.Mutex
	state     int
	errKind   int
	latest    update.Release
	asset     update.Asset
	hasAsset  bool
	pct       int
	dirty     bool   // l'état a changé : reconstruire la page des réglages
	launch    string // .msi téléchargé, à lancer depuis le fil de l'interface
	lastCheck time.Time
}

func updState() (state int, rel update.Release, hasAsset bool) {
	upd.mu.Lock()
	defer upd.mu.Unlock()
	return upd.state, upd.latest, upd.hasAsset
}

// ---------------------------------------------------------------------------
// Recherche
// ---------------------------------------------------------------------------

func fetchLatest() (update.Release, int) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", update.LatestURL(updateRepo), nil)
	if err != nil {
		return update.Release{}, errOther
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Lumen/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return update.Release{}, errNet
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 404:
		return update.Release{}, errNone
	case 403, 429:
		return update.Release{}, errRate
	default:
		return update.Release{}, errOther
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return update.Release{}, errNet
	}
	rel, err := update.ParseRelease(body)
	if err != nil || rel.Tag == "" || rel.Draft {
		return update.Release{}, errOther
	}
	return rel, -1
}

// updateCheck cherche une nouvelle version en arrière-plan (une seule recherche à la fois).
func updateCheck() {
	upd.mu.Lock()
	if upd.state == updChecking || upd.state == updDownloading || upd.state == updInstalling {
		upd.mu.Unlock()
		return
	}
	upd.state, upd.lastCheck, upd.dirty = updChecking, time.Now(), true
	upd.mu.Unlock()

	go func() {
		rel, errKind := fetchLatest()
		upd.mu.Lock()
		defer upd.mu.Unlock()
		upd.dirty = true
		if errKind >= 0 {
			upd.state, upd.errKind = updError, errKind
			return
		}
		upd.latest = rel
		if update.Newer(rel.Tag, appVersion) {
			upd.asset, upd.hasAsset = rel.AssetFor(update.ArchName(runtime.GOARCH))
			upd.state = updAvailable
		} else {
			upd.state = updUpToDate
		}
	}()
}

// updateStartup : recherche automatique au démarrage du programme.
func updateStartup() {
	if cfg.AutoUpdate {
		updateCheck()
	}
}

// ---------------------------------------------------------------------------
// Téléchargement et installation
// ---------------------------------------------------------------------------

func downloadMSI(a update.Asset) (string, error) {
	// Le fichier est lancé avec les droits d'administrateur : il doit venir d'une release du dépôt.
	if !strings.HasPrefix(a.URL, "https://github.com/"+updateRepo+"/releases/download/") {
		return "", errors.New("adresse de téléchargement inattendue")
	}
	dir := filepath.Join(os.TempDir(), "Lumen-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(a.Name))
	tmp := dest + ".part"

	client := &http.Client{Timeout: 15 * time.Minute}
	req, err := http.NewRequest("GET", a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Lumen/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errors.New("téléchargement refusé : " + resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}

	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return "", werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if total > 0 {
				upd.mu.Lock()
				upd.pct = int(done * 100 / total)
				upd.mu.Unlock()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if a.Size > 0 && done != a.Size {
		os.Remove(tmp)
		return "", errors.New("fichier incomplet")
	}
	if strings.HasPrefix(a.Digest, "sha256:") && !strings.EqualFold(strings.TrimPrefix(a.Digest, "sha256:"), hex.EncodeToString(h.Sum(nil))) {
		os.Remove(tmp)
		return "", errors.New("empreinte du fichier incorrecte")
	}
	os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// updateDownload télécharge l'installateur de la version proposée, puis le fait lancer.
func updateDownload() {
	upd.mu.Lock()
	if upd.state != updAvailable || !upd.hasAsset {
		upd.mu.Unlock()
		return
	}
	upd.state, upd.pct, upd.dirty = updDownloading, 0, true
	a := upd.asset
	upd.mu.Unlock()

	go func() {
		path, err := downloadMSI(a)
		upd.mu.Lock()
		defer upd.mu.Unlock()
		upd.dirty = true
		if err != nil {
			upd.state, upd.errKind = updError, errDownload
			return
		}
		upd.state, upd.launch = updInstalling, path
	}()
}

// updateInstall ouvre l'installateur téléchargé (sur le fil de l'interface), comme un
// double-clic dans l'Explorateur : c'est Windows Installer qui demande l'autorisation
// d'administrateur et affiche sa progression. Il ferme Lumen puis le relance.
func updateInstall(path string) {
	r, _, _ := pShellExecute.Call(0, ptr(utf16("open")), ptr(utf16(path)), 0, 0, 1)
	if r <= 32 { // impossible à ouvrir
		upd.mu.Lock()
		upd.state, upd.errKind, upd.dirty = updError, errInstall, true
		upd.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// Interface
// ---------------------------------------------------------------------------

// updateRefreshUI reconstruit la page des réglages (les boutons dépendent de l'état).
func updateRefreshUI() {
	if st.hwnd == 0 {
		return
	}
	buildPages()
	sLayout()
	pInvalidateRect.Call(st.hwnd, 0, 0)
}

// updateTick est appelé toutes les 500 ms sur le fil de l'interface.
func updateTick() {
	upd.mu.Lock()
	launch, dirty := upd.launch, upd.dirty
	upd.launch, upd.dirty = "", false
	downloading := upd.state == updDownloading
	due := cfg.AutoUpdate && !upd.lastCheck.IsZero() && time.Since(upd.lastCheck) > updateEvery &&
		(upd.state == updUpToDate || upd.state == updError || upd.state == updAvailable)
	upd.mu.Unlock()

	if launch != "" {
		updateInstall(launch)
		dirty = true
	}
	if dirty {
		updateRefreshUI()
	} else if downloading && st.hwnd != 0 {
		pInvalidateRect.Call(st.hwnd, 0, 0) // la progression avance
	}
	if due {
		updateCheck()
	}
}

func updVersionLabel() string {
	if !update.Valid(appVersion) {
		return tr("version de développement", "development build")
	}
	return strings.TrimLeft(appVersion, "vV")
}

func updLatestLabel() string {
	_, rel, _ := updState()
	if rel.Tag == "" {
		return "—"
	}
	return strings.TrimLeft(rel.Tag, "vV")
}

func updStatusLabel() string {
	upd.mu.Lock()
	state, kind, pct, rel, hasAsset := upd.state, upd.errKind, upd.pct, upd.latest, upd.hasAsset
	upd.mu.Unlock()
	switch state {
	case updChecking:
		return tr("Recherche en cours…", "Checking…")
	case updUpToDate:
		return tr("Lumen est à jour", "Lumen is up to date")
	case updAvailable:
		if !hasAsset {
			return tr("Nouvelle version, mais sans installateur pour ce PC", "New version, but no installer for this PC")
		}
		return tr("Mise à jour disponible : ", "Update available: ") + strings.TrimLeft(rel.Tag, "vV")
	case updDownloading:
		return tr("Téléchargement… ", "Downloading… ") + itoa(pct) + " %"
	case updInstalling:
		return tr("Installation en cours…", "Installing…")
	case updError:
		switch kind {
		case errNone:
			return tr("Aucune version publiée pour l’instant", "No release published yet")
		case errNet:
			return tr("Impossible de se connecter à GitHub", "Can’t reach GitHub")
		case errRate:
			return tr("GitHub limite les requêtes : réessayez plus tard", "GitHub is rate limiting: try again later")
		case errDownload:
			return tr("Le téléchargement a échoué", "The download failed")
		case errInstall:
			return tr("Installation impossible", "Installation failed")
		}
		return tr("Erreur pendant la recherche", "Error while checking")
	}
	return tr("Pas encore vérifié", "Not checked yet")
}

// updPageName : le nom de la page, avec une pastille quand une mise à jour attend.
func updPageName() string {
	name := tr("Mises à jour", "Updates")
	if s, _, _ := updState(); s == updAvailable {
		name += "  ●"
	}
	return name
}

func updPageRows() []sRow {
	state, rel, hasAsset := updState()
	rows := []sRow{
		rInfo(tr("Version installée", "Installed version"), updVersionLabel),
		rInfo(tr("Dernière version publiée", "Latest published version"), updLatestLabel),
		rInfo(tr("État", "Status"), updStatusLabel),
		rToggle(tr("Rechercher automatiquement les mises à jour", "Check for updates automatically"), &cfg.AutoUpdate),
		rHeader(tr("Actions", "Actions")),
		rButton(tr("Rechercher les mises à jour", "Check for updates"), func() {
			updateCheck()
			updateRefreshUI()
		}),
	}
	if state == updAvailable && hasAsset {
		rows = append(rows, rButton(
			tr("Télécharger et installer la version ", "Download and install version ")+strings.TrimLeft(rel.Tag, "vV"),
			func() {
				updateDownload()
				updateRefreshUI()
			}))
	}
	page := rel.PageURL
	if page == "" {
		page = update.PageURL(updateRepo)
	}
	rows = append(rows,
		rButton(tr("Voir les versions sur GitHub", "See releases on GitHub"), func() { openURI(page) }),
		rHeader(tr("L’installation demande l’autorisation d’administrateur.", "Installing asks for administrator permission.")),
	)
	return rows
}
