package domain

import (
	"fmt"
	"strings"
)

// Código de zona: prefijo derivado del nombre + número secuencial por prefijo (NORTE-001).
// La regla es la misma en el frontend (features/zones/code.ts); cambiarla aquí exige cambiarla allá.

const (
	codePrefixMaxLength = 6
	fallbackCodePrefix  = "ZONA"
	codeNumberWidth     = 3
)

// genericZoneWords describen el tipo de zona y no la distinguen, así que no entran al prefijo.
var genericZoneWords = map[string]bool{"ZONA": true, "SECTOR": true, "AREA": true}

// accentReplacer quita las tildes y la ñ del español antes de dejar solo letras ASCII.
var accentReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N",
)

// CodePrefixFromName toma la primera palabra distintiva del nombre, en mayúsculas, sin acentos y
// solo con letras A-Z, de hasta seis letras. "Zona Norte" -> "NORTE"; "Zona" -> "ZONA".
func CodePrefixFromName(name string) string {
	for _, word := range strings.Fields(accentReplacer.Replace(name)) {
		letters := onlyASCIILetters(strings.ToUpper(word))
		if letters == "" || genericZoneWords[letters] {
			continue
		}
		if len(letters) > codePrefixMaxLength {
			letters = letters[:codePrefixMaxLength]
		}
		return letters
	}
	return fallbackCodePrefix
}

// FormatCode arma el código con el número secuencial: FormatCode("NORTE", 1) -> "NORTE-001".
func FormatCode(prefix string, number int) string {
	return fmt.Sprintf("%s-%0*d", prefix, codeNumberWidth, number)
}

func onlyASCIILetters(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
