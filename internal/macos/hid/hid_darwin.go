//go:build darwin && cgo

package hid

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>
#include <IOKit/IOReturn.h>
#include <stdint.h>
#include <stdlib.h>

static int dockwarden_hid_open(uint32_t vendor_id, uint32_t product_id, void **out_device, void **out_manager) {
	IOHIDManagerRef manager = IOHIDManagerCreate(kCFAllocatorDefault, kIOHIDOptionsTypeNone);
	if (manager == NULL) {
		return kIOReturnNoMemory;
	}
	CFMutableDictionaryRef matching = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks);
	if (matching == NULL) {
		CFRelease(manager);
		return kIOReturnNoMemory;
	}
	CFNumberRef vendor = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &vendor_id);
	CFNumberRef product = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &product_id);
	if (vendor == NULL || product == NULL) {
		if (vendor != NULL) CFRelease(vendor);
		if (product != NULL) CFRelease(product);
		CFRelease(matching);
		CFRelease(manager);
		return kIOReturnNoMemory;
	}
	CFDictionarySetValue(matching, CFSTR(kIOHIDVendorIDKey), vendor);
	CFDictionarySetValue(matching, CFSTR(kIOHIDProductIDKey), product);
	CFRelease(vendor);
	CFRelease(product);

	IOHIDManagerSetDeviceMatching(manager, matching);
	IOReturn status = IOHIDManagerOpen(manager, kIOHIDOptionsTypeNone);
	if (status != kIOReturnSuccess) {
		CFRelease(matching);
		CFRelease(manager);
		return status;
	}
	CFSetRef devices = IOHIDManagerCopyDevices(manager);
	if (devices == NULL || CFSetGetCount(devices) == 0) {
		if (devices != NULL) CFRelease(devices);
		IOHIDManagerClose(manager, kIOHIDOptionsTypeNone);
		CFRelease(matching);
		CFRelease(manager);
		return kIOReturnNotFound;
	}

	CFIndex count = CFSetGetCount(devices);
	IOHIDDeviceRef *values = calloc((size_t)count, sizeof(*values));
	if (values == NULL) {
		CFRelease(devices);
		IOHIDManagerClose(manager, kIOHIDOptionsTypeNone);
		CFRelease(matching);
		CFRelease(manager);
		return kIOReturnNoMemory;
	}
	CFSetGetValues(devices, (const void **)values);
	status = kIOReturnNotFound;
	for (CFIndex index = 0; index < count; index++) {
		IOReturn open_status = IOHIDDeviceOpen(values[index], kIOHIDOptionsTypeNone);
		if (open_status == kIOReturnSuccess) {
			CFRetain(values[index]);
			*out_device = values[index];
			*out_manager = manager;
			status = kIOReturnSuccess;
			break;
		}
		status = open_status;
	}
	free(values);
	CFRelease(devices);
	if (status != kIOReturnSuccess) {
		IOHIDManagerClose(manager, kIOHIDOptionsTypeNone);
	}
	CFRelease(matching);
	if (status != kIOReturnSuccess) {
		CFRelease(manager);
	}
	return status;
}

static int dockwarden_hid_set_report(void *raw_device, const uint8_t *report, size_t length) {
	return IOHIDDeviceSetReport(
		(IOHIDDeviceRef)raw_device,
		kIOHIDReportTypeOutput,
		0,
		report,
		length);
}

static int dockwarden_hid_get_report(void *raw_device, uint8_t *report, CFIndex *length) {
	return IOHIDDeviceGetReport(
		(IOHIDDeviceRef)raw_device,
		kIOHIDReportTypeInput,
		0,
		report,
		length);
}

static void dockwarden_hid_close(void *raw_device, void *raw_manager) {
	if (raw_device == NULL) return;
	IOHIDDeviceClose((IOHIDDeviceRef)raw_device, kIOHIDOptionsTypeNone);
	CFRelease((IOHIDDeviceRef)raw_device);
	if (raw_manager != NULL) {
		IOHIDManagerClose((IOHIDManagerRef)raw_manager, kIOHIDOptionsTypeNone);
		CFRelease((IOHIDManagerRef)raw_manager);
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	vendorDell uint32 = 0x413c
	reportSize        = 192
)

type Device struct {
	raw     unsafe.Pointer
	manager unsafe.Pointer
}

func Open(productID uint16) (*Device, error) {
	var raw unsafe.Pointer
	var manager unsafe.Pointer
	status := C.dockwarden_hid_open(C.uint32_t(vendorDell), C.uint32_t(productID), &raw, &manager)
	if status != C.kIOReturnSuccess {
		return nil, fmt.Errorf("cannot open Dell HID interface 413c:%04x: IOKit 0x%x", productID, uint32(status))
	}
	return &Device{raw: raw, manager: manager}, nil
}

func (d *Device) SetOutputReport(report []byte) error {
	if d == nil || d.raw == nil {
		return fmt.Errorf("HID device is closed")
	}
	if len(report) != reportSize {
		return fmt.Errorf("HID output report length %d, want %d", len(report), reportSize)
	}
	status := C.dockwarden_hid_set_report(d.raw, (*C.uint8_t)(unsafe.Pointer(&report[0])), C.size_t(len(report)))
	if status != C.kIOReturnSuccess {
		return fmt.Errorf("cannot set Dell HID output report: IOKit 0x%x", uint32(status))
	}
	return nil
}

func (d *Device) GetInputReport(report []byte) error {
	if d == nil || d.raw == nil {
		return fmt.Errorf("HID device is closed")
	}
	if len(report) != reportSize {
		return fmt.Errorf("HID input report length %d, want %d", len(report), reportSize)
	}
	length := C.CFIndex(len(report))
	status := C.dockwarden_hid_get_report(d.raw, (*C.uint8_t)(unsafe.Pointer(&report[0])), &length)
	if status != C.kIOReturnSuccess {
		return fmt.Errorf("cannot get Dell HID input report: IOKit 0x%x", uint32(status))
	}
	if int(length) < len(report) {
		return fmt.Errorf("Dell HID input report length %d, want %d", int(length), len(report))
	}
	return nil
}

func (d *Device) Close() {
	if d == nil || d.raw == nil {
		return
	}
	C.dockwarden_hid_close(d.raw, d.manager)
	d.raw = nil
	d.manager = nil
}
