package dockgfx

// Petits outils de dessin exportés, réutilisés par les autres paquets
// (le Launchpad) pour dessiner dans un tampon de pixels prémultipliés.

// DrawSprite dessine le sprite dans le rectangle r (interpolation bilinéaire).
func DrawSprite(buf []uint32, W, H int, s *Sprite, r Rect) { drawSprite(buf, W, H, s, r) }

// DrawDisc dessine un disque lissé de couleur prémultipliée.
func DrawDisc(buf []uint32, W, H int, cx, cy, rad float64, color uint32) {
	drawDisc(buf, W, H, cx, cy, rad, color)
}

// Over compose src par-dessus dst (tous deux prémultipliés).
func Over(dst, src uint32) uint32 { return over(dst, src) }

// Premul fabrique un pixel prémultiplié à partir d'une couleur et d'une opacité (0 à 1).
func Premul(r, g, b uint8, alpha float64) uint32 { return premul(r, g, b, alpha) }

// ScaleColor multiplie un pixel prémultiplié par un coefficient de couverture (0 à 1).
func ScaleColor(c uint32, k float64) uint32 { return scaleColor(c, k) }

// SDRoundRect donne la distance signée d'un point à un rectangle arrondi
// (négative à l'intérieur).
func SDRoundRect(px, py float64, r Rect, rad float64) float64 { return sdRoundRect(px, py, r, rad) }

// Clamp01 borne une valeur entre 0 et 1.
func Clamp01(v float64) float64 { return clamp01(v) }
