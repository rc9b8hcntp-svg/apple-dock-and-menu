# Menu Windows

Un menu système façon « menu Apple » pour Windows, avec un Dock comme sur macOS.

**Une barre fine en haut de l'écran** (elle réserve sa place : les fenêtres agrandies s'arrêtent dessous) :

- **Bouton menu** (à gauche, ou `Ctrl+Alt+M`) : À propos de ce PC · Paramètres système · Microsoft Store · Éléments récents · Forcer à quitter · Mettre en veille · Redémarrer · Éteindre · Verrouiller l'écran · Fermer la session · Quitter
- **Applications** : les programmes du menu Démarrer, classés par lettre, plus « Toutes les applications… » (y compris celles du Microsoft Store)
- **Nom de la fenêtre active** en gras, puis la **date et l'heure** à droite

**Un Dock flottant en bas de l'écran** (la barre des tâches « à la Mac ») :

- vos applications épinglées, puis celles qui sont ouvertes (un **point** sous chaque application active), puis la **Corbeille** ;
- les icônes **grossissent sous la souris**, avec une étiquette au survol ;
- **clic** : ouvre l'application ou ramène sa fenêtre devant (avec plusieurs fenêtres, chaque clic passe à la suivante) ;
- **clic droit** : Afficher / Réduire / Fermer, **Garder dans le Dock** / **Retirer du Dock** ; sur la Corbeille : l'ouvrir ou la vider ;
- la liste des applications épinglées est enregistrée dans `%APPDATA%\MenuWindows\dock.txt` (un chemin par ligne, suivi si besoin d'une tabulation et de ce qu'il faut lancer) ; supprimez ce fichier pour revenir à la liste par défaut ;
- il se cache quand une application passe en plein écran et se pose juste au-dessus de la barre de Windows (masquez celle-ci automatiquement pour retrouver un écran de Mac : Paramètres → Personnalisation → Barre des tâches → Comportements).

**Aucun ancien menu Windows** : tous les menus (menu principal, Applications, clic droit sur une icône du Dock) et la confirmation avant Redémarrer / Éteindre / Fermer la session sont dessinés par le programme, dans le style de la barre (fond sombre, coins arrondis, surbrillance bleue, sous-menus). Au clavier : flèches, Entrée, Échap, et une lettre pour sauter à l'entrée correspondante (pratique dans Applications). Les longues listes défilent à la molette ou en plaçant la souris sur les flèches en haut et en bas.

Le programme est écrit en Go, sans aucune dépendance, et empaqueté en MSI avec WiX. Il se lance à chaque ouverture de session et ne remplace pas la barre des tâches de Windows. Le dessin du Dock est un petit paquet indépendant de Windows (`internal/dockgfx`), testé avec `go test ./internal/...`.

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
