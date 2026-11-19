package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// DarkSoccerTheme tema oscuro personalizado para futbol predictor
type DarkSoccerTheme struct{}

var _ fyne.Theme = (*DarkSoccerTheme)(nil)

// Colores personalizados
var (
	// Fondos
	colorBackground = color.NRGBA{R: 18, G: 18, B: 18, A: 255} // #121212
	colorSurface    = color.NRGBA{R: 30, G: 30, B: 30, A: 255} // #1E1E1E
	colorCard       = color.NRGBA{R: 37, G: 37, B: 37, A: 255} // #252525

	// Texto
	colorTextPrimary   = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Blanco
	colorTextSecondary = color.NRGBA{R: 176, G: 176, B: 176, A: 255} // #B0B0B0

	// Acentos futbol
	colorAccentGreen = color.NRGBA{R: 76, G: 175, B: 80, A: 255}  // Verde campo
	colorAccentBlue  = color.NRGBA{R: 33, G: 150, B: 243, A: 255} // Azul cielo
	colorAccentGold  = color.NRGBA{R: 255, G: 193, B: 7, A: 255}  // Dorado balón

	// Estados
	colorSuccess = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colorWarning = color.NRGBA{R: 255, G: 152, B: 0, A: 255}
	colorError   = color.NRGBA{R: 244, G: 67, B: 54, A: 255}
)

func (m DarkSoccerTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameButton:
		return colorSurface
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 60, G: 60, B: 60, A: 255}
	case theme.ColorNameDisabled:
		return colorTextSecondary
	case theme.ColorNameError:
		return colorError
	case theme.ColorNameFocus:
		return colorAccentBlue
	case theme.ColorNameForeground:
		return colorTextPrimary
	case theme.ColorNameHover:
		return color.NRGBA{R: 50, G: 50, B: 50, A: 255}
	case theme.ColorNameInputBackground:
		return colorCard
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	case theme.ColorNamePlaceHolder:
		return colorTextSecondary
	case theme.ColorNamePrimary:
		return colorAccentGreen // Botones principales en verde
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 100}
	case theme.ColorNameSuccess:
		return colorSuccess
	case theme.ColorNameWarning:
		return colorWarning
	default:
		return theme.DarkTheme().Color(name, variant)
	}
}

func (m DarkSoccerTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

func (m DarkSoccerTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

func (m DarkSoccerTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameScrollBar:
		return 12
	case theme.SizeNameScrollBarSmall:
		return 6
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameSubHeadingText:
		return 18
	default:
		return theme.DarkTheme().Size(name)
	}
}
