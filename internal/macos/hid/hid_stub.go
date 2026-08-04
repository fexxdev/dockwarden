//go:build !darwin || !cgo

package hid

import "fmt"

type Device struct{}

func Open(productID uint16) (*Device, error) {
	return nil, fmt.Errorf("native macOS HID access requires darwin with cgo")
}

func (d *Device) SetOutputReport(report []byte) error {
	return fmt.Errorf("native macOS HID access is unavailable")
}

func (d *Device) GetInputReport(report []byte) error {
	return fmt.Errorf("native macOS HID access is unavailable")
}

func (d *Device) Close() {}
