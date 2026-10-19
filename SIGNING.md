# Signer Lumen gratuitement (SignPath Foundation)

Une signature de code fait disparaître la plupart des fausses alertes de Windows Defender
(`Trojan:Win32/Bearfoos.B!ml` et cie) et de SmartScreen. **SignPath Foundation** signe
gratuitement les projets open source. **L'acceptation n'est pas garantie** : elle est
décidée au cas par cas, après un contrôle de la réputation du projet.

Ce dépôt est déjà prêt : le workflow `.github/workflows/build.yml` signe les trois `lumen.exe`
(x64, x86, arm64) **dès que la variable `SIGNPATH_ORGANIZATION_ID` existe**. Tant qu'elle
n'existe pas, tout fonctionne comme avant, sans signature.

## Ce que SignPath Foundation exige

(d'après <https://signpath.org/terms>, à relire avant de postuler)

- une licence open source reconnue par l'OSI, sans composant propriétaire : **`LICENSE` (MIT) est
  ajouté** ; change-le si tu préfères une autre licence open source ;
- un projet activement maintenu et **déjà publié** sous la forme à signer (les releases GitHub) ;
- des binaires construits **automatiquement** depuis le dépôt (GitHub Actions) ;
- **une approbation manuelle pour chaque release** signée ;
- le nom du produit et la version dans l'exécutable : **fait** (`assets/make_syso.py` les écrit
  dans `lumen.exe`) ;
- l'authentification à deux facteurs (MFA) sur les comptes GitHub et SignPath de l'équipe ;
- aucun logiciel malveillant ou indésirable ;
- une page d'accueil qui contient la « politique de signature » (texte plus bas).

## Étapes

1. **Pousse ce dépôt** (avec `LICENSE`) et laisse le workflow publier au moins une release.
2. **Active l'authentification à deux facteurs** sur ton compte GitHub.
3. **Postule** sur <https://signpath.org/> (« Apply for a free certificate ») en indiquant l'adresse
   du dépôt. Attends la réponse.
4. Une fois accepté, dans **SignPath** :
   - ajoute le *Trusted Build System* « GitHub.com » à ton organisation et relie-le au projet ;
   - crée la configuration d'artefact `exes` avec le XML ci-dessous ;
   - crée la politique de signature `release-signing` (avec approbation manuelle) ;
   - crée un utilisateur « API » avec le droit de soumettre, et copie son jeton.
5. Dans **GitHub** (Settings > Secrets and variables > Actions) :
   - secret `SIGNPATH_API_TOKEN` : le jeton ;
   - variable `SIGNPATH_ORGANIZATION_ID` : l'identifiant de ton organisation SignPath ;
   - (facultatif) `SIGNPATH_PROJECT_SLUG` (défaut `lumen`), `SIGNPATH_POLICY_SLUG` (défaut
     `release-signing`), `SIGNPATH_ARTIFACT_SLUG` (défaut `exes`) ;
   - installe aussi l'application GitHub « SignPath » sur le dépôt.
6. À chaque push (hors pull request), le workflow envoie les trois `lumen.exe` à SignPath : **approuve
   la demande** dans SignPath (le job attend jusqu'à 2 h), puis les MSI sont construits avec les
   exécutables signés. Pour ne signer que les versions marquées, remplace la condition du job `sign`
   par `startsWith(github.ref, 'refs/tags/v')`.
7. Colle le texte de la politique de signature dans le `README.md` (voir plus bas).

## Configuration d'artefact SignPath (`exes`)

Le workflow envoie un zip qui contient `exe-x64/lumen.exe`, `exe-x86/lumen.exe` et
`exe-arm64/lumen.exe`.

```xml
<artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
  <zip-file>
    <directory path="exe-x64">
      <pe-file path="lumen.exe" product-name="Lumen"><authenticode-sign/></pe-file>
    </directory>
    <directory path="exe-x86">
      <pe-file path="lumen.exe" product-name="Lumen"><authenticode-sign/></pe-file>
    </directory>
    <directory path="exe-arm64">
      <pe-file path="lumen.exe" product-name="Lumen"><authenticode-sign/></pe-file>
    </directory>
  </zip-file>
</artifact-configuration>
```

## Texte à ajouter au README après acceptation

> **Politique de signature de code.** Signature de code gratuite fournie par
> [SignPath.io](https://about.signpath.io/), certificat de la
> [SignPath Foundation](https://signpath.org/). Rôles : auteur, relecteur et approbateur : Maxence.
> **Confidentialité.** Lumen n'envoie aucune donnée personnelle. Il contacte uniquement GitHub
> (`api.github.com`) pour chercher une mise à jour ; cette recherche se désactive dans
> Réglages > Mises à jour.

## Et le MSI ?

Seuls les `lumen.exe` sont signés (c'est eux que Defender analyse, et une seule approbation suffit).
Le MSI peut être signé de la même façon plus tard avec une seconde demande de signature
(configuration d'artefact `<msi-file>`).
