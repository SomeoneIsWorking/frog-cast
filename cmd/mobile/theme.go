package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The palette of the app icon: deep navy with a sky-blue accent.
var (
	colorBackground = color.NRGBA{0x0d, 0x13, 0x22, 0xff}
	colorSurface    = color.NRGBA{0x17, 0x20, 0x33, 0xff}
	colorRaised     = color.NRGBA{0x21, 0x2c, 0x42, 0xff}
	colorBorder     = color.NRGBA{0x2b, 0x37, 0x50, 0xff}
	colorAccent     = color.NRGBA{0x60, 0xa5, 0xfa, 0xff}
	colorAccentSoft = color.NRGBA{0x60, 0xa5, 0xfa, 0x26}
	colorText       = color.NRGBA{0xf1, 0xf5, 0xf9, 0xff}
	colorMuted      = color.NRGBA{0x94, 0xa3, 0xb8, 0xff}
	colorFaint      = color.NRGBA{0x64, 0x74, 0x8b, 0xff}
	colorDanger     = color.NRGBA{0xf8, 0x71, 0x71, 0xff}
	colorSuccess    = color.NRGBA{0x4a, 0xde, 0x80, 0xff}
)

// cardRadius rounds cards, sheets and the navigation pill.
const cardRadius = 14

// frogTheme is the app's one theme, dark whatever the system asks for.
type frogTheme struct{}

var _ fyne.Theme = frogTheme{}

func (frogTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return colorRaised
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameHeaderBackground:
		return colorSurface
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameDisabledButton:
		return colorSurface
	case theme.ColorNameDisabled:
		return colorMuted
	case theme.ColorNamePlaceHolder:
		return colorFaint
	case theme.ColorNameForeground:
		return colorText
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return colorAccent
	case theme.ColorNameForegroundOnPrimary:
		return colorBackground
	case theme.ColorNameSelection:
		return colorAccentSoft
	case theme.ColorNameHover:
		return color.NRGBA{0xff, 0xff, 0xff, 0x0d}
	case theme.ColorNamePressed:
		return color.NRGBA{0xff, 0xff, 0xff, 0x1a}
	case theme.ColorNameScrollBar:
		return color.NRGBA{0xff, 0xff, 0xff, 0x33}
	case theme.ColorNameError:
		return colorDanger
	case theme.ColorNameSuccess:
		return colorSuccess
	case theme.ColorNameShadow:
		return color.NRGBA{0, 0, 0, 0x66}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (frogTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (frogTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (frogTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 15
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameSubHeadingText:
		return 18
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 10
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}
