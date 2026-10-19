# Lumen

Un menu système façon « menu Apple » pour Windows, avec un Dock comme sur macOS.

**Nom et icône** : l'application s'appelait « Menu Windows » ; elle s'appelle maintenant **Lumen**. Au premier lancement, elle reprend toute seule les réglages, le Dock, les icônes personnalisées (dossier `%APPDATA%\MenuWindows` devenu `%APPDATA%\Lumen`) et le démarrage automatique de l'ancienne version, et l'installateur remplace l'ancienne installation. L'icône (un carré arrondi bleu avec la barre de menus, un soleil et le Dock) est intégrée à `lumen.exe`, au raccourci du menu Démarrer, à la zone de notification et à la liste des applications installées. Elle est dessinée par `assets/make_icon.py` ; `python3 assets/make_syso.py` régénère `assets/lumen.ico` et les fichiers `rsrc_windows_*.syso` que Go glisse dans l'exécutable (il faut Pillow).

**Une barre fine en haut de l'écran, comme la barre des menus de macOS** (24 px, elle réserve sa place : les fenêtres agrandies s'arrêtent dessous) :

- **Symbole du menu** (à gauche, ou `Ctrl+Alt+M`) : À propos de ce PC (une fenêtre façon « À propos de ce Mac » : système et version, processeur, mémoire, graphiques, disque, durée depuis le démarrage) · Paramètres système · Microsoft Store · Éléments récents · Forcer à quitter (une fenêtre façon macOS : liste des applications ouvertes avec leur icône, « ne répond pas » en rouge, bouton rouge « Forcer à quitter » avec confirmation ; l'Explorateur se « relance » ; raccourci `Ctrl+Alt+Échap`) · Mettre en veille · Redémarrer · Éteindre · Verrouiller l'écran · Fermer la session · Quitter
- **Nom de l'application active** en gras (« Bloc-notes », « Google Chrome »… ; « Bureau » quand aucune fenêtre n'est active), puis le menu **Applications** : les programmes du menu Démarrer, classés par lettre, plus « Toutes les applications… » (y compris celles du Microsoft Store)
- **Masquage automatique de la barre** (Réglages > Barre) : la barre ne réserve plus de place, remonte hors de l'écran et redescend quand la souris touche le bord haut ; elle reste affichée tant qu'un menu ou un panneau (Spotlight, Centre de contrôle, calendrier…) est ouvert, et ne s'affiche pas par-dessus un jeu ou une présentation en plein écran. Le raccourci `Ctrl+Alt+M` la fait redescendre ;
- **Menus Fichier et Édition**, comme dans la barre de macOS (visibles quand une application est active) : Nouveau, Ouvrir, Enregistrer, Imprimer… et Annuler, Couper, Copier, Coller, Tout sélectionner, Rechercher, Émojis et symboles. Windows ne permet pas de lire les menus d'une autre application : chaque entrée envoie à l'application active le raccourci clavier équivalent (`Ctrl+N`, `Ctrl+C`…) ;
- **Clavier** : le code de la langue du clavier actif (« FR », « EN »…) s'affiche à droite quand plusieurs dispositions sont installées ; son menu les liste et en active une ;
- **Menu Fenêtre**, comme sur macOS : Réduire · Zoom (ou Restaurer) · Centrer · Placer à gauche / à droite de l'écran · Fermer la fenêtre · Tout ramener au premier plan, puis la liste des fenêtres ouvertes (la fenêtre active est cochée, un clic en ramène une devant)
- **Menu de l'application** (clic sur son nom en gras) : À propos de… (propriétés du programme) · Masquer · Masquer les autres · Tout afficher · Quitter
- **Menu du symbole** : À propos de ce PC · Paramètres · Éléments récents (sous-menu des documents récents) · Forcer à quitter · Veille · Redémarrer · Éteindre · Verrouiller · Fermer la session de *votre nom*
- **Icônes d'état à droite**, comme sur macOS : **batterie** (avec le pourcentage, verte en charge, rouge sous 20 % ; masquée sur un PC de bureau), **Wi-Fi** (grisé quand la carte n'est pas connectée, masqué s'il n'y a pas de carte Wi-Fi), **son** (désactivé par défaut), **recherche**, **centre de contrôle**, puis la **date et l'heure** (« jeu. 15 oct. 14:32 »). **Chaque icône ouvre un vrai menu déroulant dans la barre** (dès l'appui, et on peut glisser d'une icône à l'autre) ; les cases à cocher et les curseurs de ces menus **modifient directement les réglages** (`settings.ini`), et la fenêtre de réglages se met à jour en même temps :
  - **Batterie** : pourcentage, source d'alimentation, temps restant ; cases « Afficher le pourcentage » et « Afficher la batterie dans la barre » ; accès à l'économiseur de batterie ;
  - **Wi-Fi** : interrupteur Wi-Fi (radio), « Réseaux préférés » et « Autres réseaux » (le réseau actuel est coché ; un clic se connecte à un réseau déjà connu, sinon ouvre les réglages Windows pour le mot de passe) ; case « Afficher le Wi-Fi dans la barre » ;
  - **Bluetooth** (icône dédiée, masquée s'il n'y a pas de radio Bluetooth) : interrupteur Bluetooth, liste des appareils appairés (coché = connecté, état réel lu par l'API Bluetooth de Windows) ; chaque appareil a un sous-menu **Connecter / Déconnecter** (Windows n'a pas d'appel direct : le programme active ou désactive les services de l'appareil, audio, mains libres, casque et HID ; après un « Déconnecter », utilisez « Connecter » du menu ou les réglages Windows pour le reconnecter), case « Afficher le Bluetooth dans la barre ». La lecture et l'activation passent par PowerShell (WinRT), en arrière-plan ;
  - **Son** : curseur de volume, Muet, case « Afficher le son dans la barre » ;
  - **Recherche = Spotlight** : une vraie fenêtre de recherche au centre en haut de l'écran (aussi avec `Ctrl+Alt+Espace`). On tape, les résultats apparaissent **groupés avec leurs icônes** : Meilleur résultat, Calculatrice (`12*3,5` donne `42`, Entrée copie le résultat), Applications (menu Démarrer, Launchpad, réglages de Lumen), Réglages Windows, Dossiers, Documents, PDF et Images (Bureau, Documents, Téléchargements, Images, Musique, Vidéos, indexés en arrière-plan), puis « Rechercher sur le Web ». Flèches ou Tab pour choisir, Entrée ou clic pour ouvrir, Échap efface puis ferme ;
  - **Centre de contrôle** : un panneau de **tuiles** comme sur macOS : Wi-Fi, Mode sombre, Dock, Launchpad, Mission Control (`Win+Tab`), **Ne pas déranger** (coupe les bannières de notification de Windows), **Bluetooth** (vrai interrupteur de la radio, comme la tuile Wi-Fi) et Capture d'écran (`Win+Maj+S`) ; une rangée **multimédia** (précédent, lecture/pause, suivant ; masquée sur un écran bas) ; des **curseurs** pour l'opacité de la barre, la **luminosité de l'écran** (portables uniquement : lue et réglée via WMI ; le curseur n'apparaît pas sur un PC de bureau ou un écran externe) et le volume ; une rangée de pastilles pour la **couleur d'accentuation** ; les boutons Réglages de Lumen et Paramètres Windows. Tout ce qui touche aux réglages est enregistré dans `settings.ini` ;
  - **Horloge** : un clic ouvre le **calendrier** (jour et date en grand, calendrier du mois avec aujourd'hui entouré, ‹ › ou molette pour changer de mois, le point revient à aujourd'hui) ; un **clic droit** ouvre les réglages de l'horloge (date, secondes, 24 h / 12 h) ;
- **Comme sur macOS, un menu s'ouvre dès que le bouton est enfoncé**, et en glissant sur un autre titre pendant qu'un menu est ouvert, on passe directement à son menu
- Comme sur macOS, un titre ne s'éclaire que lorsque son menu est ouvert, pas au simple survol.

**Coins actifs, flou et widgets** :

- **Fenêtre Réglages façon « Réglages Système »** : sans bordure, avec les boutons rouge (fermer) / jaune (réduire) / gris, qui se grisent quand la fenêtre n'est pas active ; barre latérale avec le profil Windows et des icônes colorées (flèches haut/bas pour changer de page) ; titre dans la barre d'outils (on déplace la fenêtre en la saisissant par là) ; cartes arrondies, interrupteurs, curseurs et contrôles segmentés ; défilement à la molette quand une page est longue. La boîte de confirmation (Redémarrer, Éteindre…) prend l'aspect d'une alerte macOS avec une icône à gauche ;
- **Boutons de fenêtre façon macOS** (Réglages > Apparence, *expérimental, désactivé par défaut*) : Windows dessine lui-même la barre de titre des autres programmes, on ne peut donc pas y déplacer les boutons. Lumen pose par-dessus une barre de titre à la macOS rattachée à chaque fenêtre : pastilles rouge (fermer), jaune (réduire) et verte (agrandir / restaurer) à gauche avec leurs symboles au survol, titre centré, grisée quand la fenêtre n'est pas active. Glisser la barre déplace la fenêtre, un double clic l'agrandit, le bord haut la redimensionne, un clic droit ouvre le menu système. Aucune fenêtre n'est modifiée. Les programmes qui dessinent eux-mêmes leur barre de titre (Chrome, Edge, Firefox, VS Code et autres applications Electron, Explorateur de fichiers, Terminal Windows, Office, Bloc-notes de Windows 11, applications du Store…) sont laissés tels quels ;
- **Coins actifs** (Réglages > Coins actifs) : quand la souris reste environ 0,3 s dans un coin de l'écran, l'action choisie se lance : Mission Control (`Win+Tab`), Launchpad, Spotlight ou afficher le bureau (`Win+D`). Elle ne se relance qu'après être sortie du coin ; rien ne se passe pendant un glisser-déposer, un menu ouvert ou un jeu en plein écran ;
- **Flou derrière la barre** (Réglages > Apparence) : le fond d'écran sous la barre est flouté et teinté de la couleur de la barre ; le curseur d'opacité règle alors la force de la teinte ;
- **Coins d'écran arrondis** (Réglages > Apparence) : quatre petits coins noirs au-dessus de tout, comme sur un Mac ; les deux du haut se posent sous la barre ;
- **Widgets sur le bureau** (Réglages > Apparence) : horloge analogique, date et batterie (sur portable) en cartes arrondies en haut à droite, derrière toutes les fenêtres ; les clics passent au travers.

- **Sélecteur d'applications façon Cmd+Tab** (Réglages > Général, désactivé par défaut) : `Alt+Tab` ouvre un panneau de grandes icônes, une par application (la plus récente d'abord). `Tab` ou `←` `→` avance, `Maj+Tab` recule, **`Q` quitte l'application choisie**, `Échap` annule, relâcher `Alt` (ou cliquer une icône) active l'application. Il remplace l'`Alt+Tab` de Windows grâce à un crochet clavier : décochez le réglage pour retrouver celui de Windows ;
- **Police** : le programme prend automatiquement **SF Pro** (la police d'Apple, à installer vous-même) si elle est présente, sinon Segoe UI Variable (Windows 11), sinon Segoe UI. Relancez le programme après l'installation de la police.

**Un Dock flottant en bas de l'écran** (la barre des tâches « à la Mac ») :

- **Barre des tâches Windows** : le réglage « Masquer la barre des tâches Windows » (Réglages > Barre) la cache pour de bon : elle ne remonte plus quand la souris touche le bord bas de l'écran, et sa place est libérée. Elle est rétablie telle qu'elle était à la fermeture du programme ou quand on décoche le réglage (si le programme s'arrêtait brutalement, la relancer puis décocher le réglage la remet)
- **Dock : masquage automatique** (Réglages > Dock) : le Dock glisse vers le bas quand la souris s'en éloigne et revient quand elle touche le bord bas de l'écran. **Rebond** : l'icône d'une application saute pendant son lancement, comme sur macOS.
- **Launchpad en deux tailles** (Réglages > Dock, ou clic droit sur son icône du Dock) : **plein écran** ou **petit**, une fenêtre centrée aux coins arrondis sur fond flou ;
- **Launchpad** : la première icône du Dock (ou `Ctrl+Alt+L`, ou le menu du symbole) ouvre une grille plein écran de toutes vos applications (celles du menu Démarrer), sur un fond flou. Tapez pour rechercher (Échap efface la recherche, puis ferme), changez de page à la molette, avec les points du bas, les flèches ou Page préc./Page suiv., Entrée ou un clic lance l'application, un clic à côté ferme. Pour changer son icône : `launchpad.png` dans le dossier des icônes ;
- **Position du Dock** (Réglages > Dock, ou clic droit sur l'icône du Launchpad) : **en bas, à gauche ou à droite** de l'écran, comme sur macOS. Le zoom, le rebond, les étiquettes (placées à côté de l'icône), le menu contextuel et le masquage automatique suivent le côté choisi ;
- **Look macOS** : la barre du Dock est en **verre dépoli** (le décor derrière est copié, flouté et teinté de clair ou de sombre selon le thème, avec un fin liseré lumineux plus marqué en haut), chaque icône a une **ombre douce**, et l'option **« Icônes en carré arrondi »** (désactivée par défaut) pose les icônes qui n'ont pas de fond sur une plaque en carré arrondi, comme les icônes de macOS, et découpe celles qui remplissent déjà leur carré. Réglages > Dock : « Fond en verre dépoli » et « Icônes en carré arrondi » ;
- vos applications épinglées, puis celles qui sont ouvertes (un **point** sous chaque application active), puis la **Corbeille** ;
- les icônes **grossissent sous la souris**, avec une étiquette au survol ;
- **clic** : ouvre l'application ou ramène sa fenêtre devant (avec plusieurs fenêtres, chaque clic passe à la suivante) ;
- **clic droit** : Afficher / Réduire / Fermer, **Garder dans le Dock** / **Retirer du Dock** ; sur la Corbeille : l'ouvrir ou la vider ;
- la liste des applications épinglées est enregistrée dans `%APPDATA%\Lumen\dock.txt` (un chemin par ligne, suivi si besoin d'une tabulation et de ce qu'il faut lancer) ; supprimez ce fichier pour revenir à la liste par défaut ;
- **icônes personnalisées** : déposez des images `.png` ou `.ico` dans `%APPDATA%\Lumen\icons` ; le nom du fichier est celui du programme sans l'extension (`notepad.png` remplace l'icône de `notepad.exe`). Pour la Corbeille : `trash.png` (vide) et `trash-full.png` (pleine). **Détection automatique d'après les mots** : le fichier n'a pas besoin de porter exactement le nom du `.exe`. Le programme compare le nom du fichier aux mots du nom du programme, de sa description (« Google Chrome ») et du titre de sa fenêtre, sans tenir compte des majuscules, des accents ni des tirets : `chrome.png` ou `google-chrome.png` conviennent pour `chrome.exe`, `bloc-notes.png` pour « Bloc-notes », `calc.png` pour « Calculatrice » (un début de mot d'au moins 4 lettres suffit). Quand plusieurs icônes conviennent, la plus précise (celle qui a le plus de mots) l'emporte, et le nom exact du `.exe` passe toujours en premier. Supprimez le fichier pour retrouver l'icône d'origine. Relancez le programme pour que le changement soit pris en compte ;
- il se cache quand une application passe en plein écran et se pose juste au-dessus de la barre de Windows (masquez celle-ci automatiquement pour retrouver un écran de Mac : Paramètres → Personnalisation → Barre des tâches → Comportements).

**Réglages** (menu du symbole → « Réglages de Lumen… ») : une fenêtre dessinée par le programme, avec six pages. Chaque changement s'applique tout de suite et s'enregistre dans `%APPDATA%\Lumen\settings.ini` (un réglage par ligne, modifiable à la main ; supprimez le fichier pour tout rétablir).

| Page | Réglages |
|---|---|
| Général | langue (automatique, français, English) · lancer à l'ouverture de session · raccourcis clavier (`Ctrl+Alt+M`, `L`, `Espace`, `Échap`) · sélecteur d'applications · ouvrir le dossier des icônes · rétablir tous les réglages |
| Apparence | thème (automatique selon Windows, clair, sombre) · couleur d'accentuation (bleu, violet, rose, rouge, orange, jaune, vert, graphite) · opacité de la barre · flou derrière la barre · widgets sur le bureau · coins d'écran arrondis |
| Barre | hauteur · nom de l'application active · menus Applications et Fenêtre · horloge : date, secondes, format 24 h / 12 h |
| Icônes d'état | batterie (et pourcentage) · Wi-Fi · son · recherche · centre de contrôle |
| Coins actifs | action pour chacun des quatre coins : aucune, Mission Control, Launchpad, Spotlight, bureau |
| Dock | afficher le Dock · taille des icônes · agrandissement · icône du Launchpad · étiquettes · point des applications ouvertes · applications ouvertes non épinglées · Corbeille · rétablir les applications du Dock |

**Aucun ancien menu Windows** : tous les menus (menu principal, Applications, clic droit sur une icône du Dock) et la confirmation avant Redémarrer / Éteindre / Fermer la session sont dessinés par le programme, dans le style de la barre (fond sombre ou clair selon le thème, coins arrondis, surbrillance de la couleur d'accentuation, sous-menus). Au clavier : flèches, Entrée, Échap, et une lettre pour sauter à l'entrée correspondante (pratique dans Applications). Les longues listes défilent à la molette ou en plaçant la souris sur les flèches en haut et en bas.

Le programme est écrit en Go, sans aucune dépendance, et empaqueté en MSI avec WiX. Il se lance à chaque ouverture de session et ne remplace pas la barre des tâches de Windows. Le dessin du Dock (`internal/dockgfx`), le Launchpad (`internal/launchpad`) et les réglages (`internal/config`) sont de petits paquets indépendants de Windows, testés avec `go test ./internal/...`.

## Obtenir le MSI

Le workflow construit **trois MSI** à chaque fois :

| Fichier | Pour quel PC |
|---|---|
| `Lumen-x64.msi` | Windows 64 bits, processeur Intel ou AMD (la plupart des PC) |
| `Lumen-arm64.msi` | Windows sur ARM (Snapdragon, Surface Pro X, PC Copilot+ ARM) |
| `Lumen-x86.msi` | Windows 32 bits |

Windows 10 et Windows 11 uniquement. Pour savoir quel fichier choisir : Paramètres → Système → À propos de ce PC → « Type du système ».

1. Déposez ce dossier dans un dépôt GitHub (branche `main`).
2. Onglet **Actions** → workflow **Build MSI** : il se lance seul à chaque push. Téléchargez les fichiers dans **Artifacts** (`Lumen-x64`, `Lumen-arm64`, `Lumen-x86`).
3. Pour publier une version téléchargeable par tout le monde, créez un tag :
   ```
   git tag v1.0.0
   git push origin v1.0.0
   ```
   Les trois MSI sont alors ajoutés à **Releases**, avec des noms fixes : le site peut donc pointer vers `releases/latest/download/Lumen-x64.msi`.

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
go build -ldflags "-s -w -H windowsgui" -o installer\lumen.exe .
dotnet tool install --global wix --version 5.0.2
cd installer
wix build Product.wxs -arch x64 -d Version=1.0.0 -o ..\Lumen-x64.msi
```

## Remarques

- Le MSI n'est pas signé : SmartScreen peut afficher un avertissement au premier lancement.
- Projet indépendant, ni affilié ni approuvé par Apple ou Microsoft.

## Site web : compte obligatoire avant de télécharger

Le site (`site/index.html`, à publier avec GitHub Pages) demande de **créer un compte ou de se connecter** (Firebase Authentication, e-mail et mot de passe) avant tout téléchargement. Une fois connecté, un encadré **explique les risques de cette version expérimentale** ; il faut cocher « J'ai lu ces risques » pour que le bouton Télécharger s'active.

Pour l'activer :

1. Console Firebase > Authentication > Méthode de connexion : activer **E-mail/Mot de passe**.
2. Authentication > Paramètres > Domaines autorisés : ajouter le domaine du site (par exemple `rc9b8hcntp-svg.github.io`).
3. Paramètres du projet > Vos applications > **Application Web** : copier la configuration et la coller dans `site/index.html` à la place des `À_REMPLIR` (objet `FIREBASE_CONFIG`, tout en haut du dernier `<script>`).

La configuration du projet Firebase `apple-dock` est déjà dans le site. Si elle contenait encore les `À_REMPLIR`, la fenêtre de connexion indique que la connexion n'est pas configurée et aucun téléchargement ne démarre. Le dépôt GitHub utilisé pour les téléchargements est `rc9b8hcntp-svg/apple-dock-and-menu` (variable `REPO`).

Attention : ce verrou concerne la page. Les fichiers MSI des « Releases » d'un dépôt GitHub public restent accessibles à qui connaît leur adresse ; pour les réserver aux comptes connectés, il faudrait les héberger dans un stockage protégé (Firebase Storage avec des règles d'accès, par exemple).
