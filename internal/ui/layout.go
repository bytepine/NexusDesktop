// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/i18n"
)

func wrappedLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return label
}

func showConfirmation(title, message string, callback func(bool), parent fyne.Window) {
	d := dialog.NewConfirm(title, message, callback, parent)
	d.SetConfirmText(i18n.T("action.continue"))
	d.SetDismissText(i18n.T("action.cancel"))
	d.Show()
}

// windowContent 将操作区固定在底部；滚动仅属于中间内容。
func windowContent(header, body fyne.CanvasObject, status *widget.Label, actions ...fyne.CanvasObject) fyne.CanvasObject {
	row := container.NewBorder(nil, nil, nil, container.NewHBox(actions...))
	if status != nil {
		row.Add(status)
	}
	footer := container.NewVBox(widget.NewSeparator(), row)
	return container.NewPadded(container.NewBorder(header, footer, nil, nil, body))
}
