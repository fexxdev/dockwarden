//go:build !darwin || !cgo

package hid

import (
	"fmt"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type Device struct{}

func Open(target domain.HIDTarget) (*Device, error) {
	_ = target
	return nil, fmt.Errorf("native macOS HID access requires darwin with cgo")
}

func (d *Device) SetOutputReport(report []byte) error {
	return fmt.Errorf("native macOS HID access is unavailable")
}

func (d *Device) GetInputReport(report []byte) error {
	return fmt.Errorf("native macOS HID access is unavailable")
}

func (d *Device) Close() {}
