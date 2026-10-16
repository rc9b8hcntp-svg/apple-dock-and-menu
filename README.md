# Menu Windows

Un menu système façon « menu Apple » pour Windows, avec un Dock comme sur macOS.

**Une barre fine en haut de l'écran, comme la barre des menus de macOS** (24 px, elle réserve sa place : les fenêtres agrandies s'arrêtent dessous) :

- **Symbole du menu** (à gauche, ou `Ctrl+Alt+M`) : À propos de ce PC · Paramètres système · Microsoft Store · Éléments récents · Forcer à quitter · Mettre en veille · Redémarrer · Éteindre · Verrouiller l'écran · Fermer la session · Quitter
- **Nom de l'application active** en gras (« Bloc-notes », « Google Chrome »… ; « Bureau » quand aucune fenêtre n'est active), puis le menu **Applications** : les programmes du menu Démarrer, classés par lettre, plus « Toutes les applications… » (y compris celles du Microsoft Store)
- **Menu Fenêtre** : Réduire · Agrandir (ou Restaurer) · Fermer la fenêtre, puis la liste des fenêtres ouvertes (un clic en ramène une devant). Un clic sur le **nom de l'application** ouvre son menu : Masquer · Fermer
- **Icônes d'état à droite**, comme sur macOS : **batterie** (avec le pourcentage, verte en charge, rouge sous 20 % ; masquée sur un PC de bureau), **Wi-Fi** (grisé quand la carte n'est pas connectée, masqué s'il n'y a pas de carte Wi-Fi), **son** (désactivé par défaut), **recherche** (`Win+S`), **centre de contrôle** (`Win+A`), puis la **date et l'heure** (« jeu. 15 oct. 14:32 », un clic ouvre le calendrier). Chaque icône ouvre le panneau Windows correspondant
- **Comme sur macOS, un menu s'ouvre dès que le bouton est enfoncé**, et en glissant sur un autre titre pendant qu'un menu est ouvert, on passe directement à son menu
- Comme sur macOS, un titre ne s'éclaire que lorsque son menu est ouvert, pas au simple survol.

**Un Dock flottant en bas de l'écran** (la barre des tâches « à la Mac ») :

- **Launchpad** : la première icône du Dock (ou `Ctrl+Alt+L`, ou le menu du symbole) ouvre une grille plein écran de toutes vos applications (celles du menu Démarrer), sur un fond flou. Tapez pour rechercher (Échap efface la recherche, puis ferme), changez de page à la molette, avec les points du bas, les flèches ou Page préc./Page suiv., Entrée ou un clic lance l'application, un clic à côté ferme. Pour changer son icône : `launchpad.png` dans le dossier des icônes ;
- vos applications épinglées, puis celles qui sont ouvertes (un **point** sous chaque application active), puis la **Corbeille** ;
- les icônes **grossissent sous la souris**, avec une étiquette au survol ;
- **clic** : ouvre l'application ou ramène sa fenêtre devant (avec plusieurs fenêtres, chaque clic passe à la suivante) ;
- **clic droit** : Afficher / Réduire / Fermer, **Garder dans le Dock** / **Retirer du Dock** ; sur la Corbeille : l'ouvrir ou la vider ;
- la liste des applications épinglées est enregistrée dans `%APPDATA%\MenuWindows\dock.txt` (un chemin par ligne, suivi si besoin d'une tabulation et de ce qu'il faut lancer) ; supprimez ce fichier pour revenir à la liste par défaut ;
- **icônes personnalisées** : déposez des images `.png` ou `.ico` dans `%APPDATA%\MenuWindows\icons` ; le nom du fichier est celui du programme sans l'extension (`notepad.png` remplace l'icône de `notepad.exe`). Pour la Corbeille : `trash.png` (vide) et `trash-full.png` (pleine). **Détection automatique d'après les mots** : le fichier n'a pas besoin de porter exactement le nom du `.exe`. Le programme compare le nom du fichier aux mots du nom du programme, de sa description (« Google Chrome ») et du titre de sa fenêtre, sans tenir compte des majuscules, des accents ni des tirets : `chrome.png` ou `google-chrome.png` conviennent pour `chrome.exe`, `bloc-notes.png` pour « Bloc-notes », `calc.png` pour « Calculatrice » (un début de mot d'au moins 4 lettres suffit). Quand plusieurs icônes conviennent, la plus précise (celle qui a le plus de mots) l'emporte, et le nom exact du `.exe` passe toujours en premier. Supprimez le fichier pour retrouver l'icône d'origine. Relancez le programme pour que le changement soit pris en compte ;
- il se cache quand une application passe en plein écran et se pose juste au-dessus de la barre de Windows (masquez celle-ci automatiquement pour retrouver un écran de Mac : Paramètres → Personnalisation → Barre des tâches → Comportements).

**Réglages** (menu du symbole → « Réglages de Menu Windows… ») : une fenêtre dessinée par le programme, avec cinq pages. Chaque changement s'applique tout de suite et s'enregistre dans `%APPDATA%\MenuWindows\settings.ini` (un réglage par ligne, modifiable à la main ; supprimez le fichier pour tout rétablir).

| Page | Réglages |
|---|---|
| Général | langue (automatique, français, English) · lancer à l'ouverture de session · raccourcis clavier (`Ctrl+Alt+M`, `Ctrl+Alt+L`) · ouvrir le dossier des icônes · rétablir tous les réglages |
| Apparence | thème (automatique selon Windows, clair, sombre) · couleur d'accentuation (bleu, violet, rose, rouge, orange, jaune, vert, graphite) · opacité de la barre |
| Barre | hauteur · nom de l'application active · menus Applications et Fenêtre · horloge : date, secondes, format 24 h / 12 h |
| Icônes d'état | batterie (et pourcentage) · Wi-Fi · son · recherche · centre de contrôle |
| Dock | afficher le Dock · taille des icônes · agrandissement · icône du Launchpad · étiquettes · point des applications ouvertes · applications ouvertes non épinglées · Corbeille · rétablir les applications du Dock |

**Aucun ancien menu Windows** : tous les menus (menu principal, Applications, clic droit sur une icône du Dock) et la confirmation avant Redémarrer / Éteindre / Fermer la session sont dessinés par le programme, dans le style de la barre (fond sombre ou clair selon le thème, coins arrondis, surbrillance de la couleur d'accentuation, sous-menus). Au clavier : flèches, Entrée, Échap, et une lettre pour sauter à l'entrée correspondante (pratique dans Applications). Les longues listes défilent à la molette ou en plaçant la souris sur les flèches en haut et en bas.

Le programme est écrit en Go, sans aucune dépendance, et empaqueté en MSI avec WiX. Il se lance à chaque ouverture de session et ne remplace pas la barre des tâches de Windows. Le dessin du Dock (`internal/dockgfx`), le Launchpad (`internal/launchpad`) et les réglages (`internal/config`) sont de petits paquets indépendants de Windows, testés avec `go test ./internal/...`.

## Obtenir le MSI

Le workflow construit **trois MSI** à chaque fois :

| Fichier | Pour quel PC |
|---|---|
| `MenuWindows-x64.msi` | Windows 64 bits, processeur Intel ou AMD (la plupart des PC) |
| `MenuWindows-arm64.msi` | Windows sur ARM (Snapdragon, Surface Pro X, PC Copilot+ ARM) |
| `MenuWindows-x86.msi` | Windows 32 bits |

Windows 10 et Windows 11 uniquement. Pour savoir quel fichier choisir : Paramètres → Système → À propos de ce PC → « Type du système ».

1. Déposez ce dossier dans un dépôt GitHub (branche `main`).
2. Onglet **Actions** → workflow **Build MSI** : il se lance seul à chaque push. Téléchargez les fichiers dans **Artifacts** (`MenuWindows-x64`, `MenuWindows-arm64`, `MenuWindows-x86`).
3. Pour publier une version téléchargeable par tout le monde, créez un tag :
   ```
   git tag v1.0.0
   git push origin v1.0.0
   ```
   Les trois MSI sont alors ajoutés à **Releases**, avec des noms fixes : le site peut donc pointer vers `releases/latest/download/MenuWindows-x64.msi`.

## Site de présentation

Le dossier `site/` contient une page statique. Dans `site/index.html`, remplacez
`VOTRE-NOM/menu-windows` par votre compte et le nom du dépôt, puis activez GitHub Pages
(Settings → Pages → Source : **GitHub Actions**). Le workflow `pages.yml` publie le site.

## Compiler en local (Windows)

Remplacez `amd64` par `386` (32 bits) ou `arm64`, et `x64` par `x86` ou `arm64` :

```
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w -H windowsgui" -o installer\menuwin.exe .
dotnet tool install --global wix --version 5.0.2
cd installer
wix build Product.wxs -arch x64 -d Version=1.0.0 -o ..\MenuWindows-x64.msi
```

## Remarques

- Le MSI n'est pas signé : SmartScreen peut afficher un avertissement au premier lancement.
- Projet indépendant, ni affilié ni approuvé par Apple ou Microsoft.
