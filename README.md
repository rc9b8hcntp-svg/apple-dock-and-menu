# Menu Windows

Un menu système façon « menu Apple » pour Windows. Une barre fine reste en haut de l'écran (elle réserve sa place : les fenêtres agrandies s'arrêtent dessous) :

- **Bouton menu** (à gauche, ou `Ctrl+Alt+M`) : À propos de ce PC · Paramètres système · Microsoft Store · Éléments récents · Forcer à quitter · Mettre en veille · Redémarrer · Éteindre · Verrouiller l'écran · Fermer la session · Quitter
- **Applications** : les programmes du menu Démarrer, classés par lettre, plus « Toutes les applications… » (y compris celles du Microsoft Store)
- **Fenêtres ouvertes** : un bouton par fenêtre, comme une barre des tâches. Clic : afficher ou réduire. Clic droit : Afficher / Réduire / Fermer
- **Date et heure** à droite

**Aucun ancien menu Windows** : tous les menus (menu principal, Applications, clic droit sur une fenêtre) et la confirmation avant Redémarrer / Éteindre / Fermer la session sont dessinés par le programme, dans le style de la barre (fond sombre, coins arrondis, surbrillance bleue, sous-menus). Au clavier : flèches, Entrée, Échap, et une lettre pour sauter à l'entrée correspondante (pratique dans Applications). Les longues listes défilent à la molette ou en plaçant la souris sur les flèches en haut et en bas.

Le programme est écrit en Go (aucune dépendance), empaqueté en MSI avec WiX, et se lance à chaque ouverture de session. Il ne remplace pas la barre des tâches de Windows : vous pouvez la garder ou la masquer automatiquement.

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
