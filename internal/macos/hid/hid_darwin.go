//go:build darwin && cgo

package hid

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>
#include <IOKit/IOReturn.h>
#include <string.h>
#include <stdint.h>
#include <stdlib.h>

static int dockwarden_hid_matches(IOHIDDeviceRef device, uint32_t location_id, const char *serial) {
	int matched_identity = 0;
	if (location_id != 0) {
		CFTypeRef raw_location = IOHIDDeviceGetProperty(device, CFSTR(kIOHIDLocationIDKey));
		int32_t actual_location = 0;
		if (raw_location != NULL && CFGetTypeID(raw_location) == CFNumberGetTypeID() &&
		    CFNumberGetValue((CFNumberRef)raw_location, kCFNumberSInt32Type, &actual_location)) {
			if ((uint32_t)actual_location != location_id) {
				return 0;
			}
			matched_identity = 1;
		}
	}
	if (serial != NULL && serial[0] != '\0') {
		CFTypeRef raw_serial = IOHIDDeviceGetProperty(device, CFSTR(kIOHIDSerialNumberKey));
		char actual_serial[256] = {0};
		if (raw_serial != NULL && CFGetTypeID(raw_serial) == CFStringGetTypeID() &&
		    CFStringGetCString((CFStringRef)raw_serial, actual_serial, sizeof(actual_serial), kCFStringEncodingUTF8)) {
			if (strcmp(actual_serial, serial) != 0) {
				return 0;
			}
			matched_identity = 1;
		}
	}
	return matched_identity;
}

static int dockwarden_hid_open(uint32_t vendor_id, uint32_t product_id, uint32_t location_id, const char *serial, void **out_device, void **out_manager, uint32_t *out_matches) {
	*out_device = NULL;
	*out_manager = NULL;
	*out_matches = 0;
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
	IOHIDDeviceRef selected = NULL;
	uint32_t matches = 0;
	for (CFIndex index = 0; index < count; index++) {
		if (dockwarden_hid_matches(values[index], location_id, serial)) {
			selected = values[index];
			matches++;
		}
	}
	*out_matches = matches;
	if (matches == 0) {
		free(values);
		CFRelease(devices);
		IOHIDManagerClose(manager, kIOHIDOptionsTypeNone);
		CFRelease(matching);
		CFRelease(manager);
		return kIOReturnNotFound;
	}
	if (matches != 1) {
		free(values);
		CFRelease(devices);
		IOHIDManagerClose(manager, kIOHIDOptionsTypeNone);
		CFRelease(matching);
		CFRelease(manager);
		return kIOReturnExclusiveAccess;
	}
	status = IOHIDDeviceOpen(selected, kIOHIDOptionsTypeNone);
	if (status == kIOReturnSuccess) {
		CFRetain(selected);
		*out_device = selected;
		*out_manager = manager;
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

typedef struct {
	IOReturn status;
	int done;
	CFIndex capacity;
	CFIndex length;
} dockwarden_hid_report_context;

static void dockwarden_hid_report_callback(
	void *context,
	IOReturn status,
	void *sender,
	IOHIDReportType type,
	uint32_t report_id,
	uint8_t *report,
	CFIndex length) {
	(void)sender;
	(void)type;
	(void)report_id;
	(void)report;
	dockwarden_hid_report_context *state = (dockwarden_hid_report_context *)context;
	state->status = status;
	if (status == kIOReturnSuccess) {
		if (length < 0 || length > state->capacity) {
			state->status = kIOReturnOverrun;
		} else {
			state->length = length;
		}
	}
	state->done = 1;
}

static IOReturn dockwarden_hid_wait_for_report(
	IOHIDDeviceRef device,
	IOHIDReportType report_type,
	const uint8_t *set_report,
	uint8_t *get_report,
	CFIndex capacity,
	CFIndex *length) {
	// fwupd uses a 2000 ms HID transaction timeout.
	const CFTimeInterval report_timeout_ms = 2000.0;
	const CFTimeInterval run_loop_timeout_seconds = 2.5;
	CFRunLoopRef run_loop = CFRunLoopGetCurrent();
	dockwarden_hid_report_context context = {
		.status = kIOReturnTimeout,
		.done = 0,
		.capacity = capacity,
		.length = 0,
	};
	IOHIDDeviceScheduleWithRunLoop(device, run_loop, kCFRunLoopDefaultMode);
	IOReturn status;
	if (report_type == kIOHIDReportTypeOutput) {
		status = IOHIDDeviceSetReportWithCallback(
			device,
			report_type,
			0,
			set_report,
			capacity,
			report_timeout_ms,
			dockwarden_hid_report_callback,
			&context);
	} else {
		status = IOHIDDeviceGetReportWithCallback(
			device,
			report_type,
			0,
			get_report,
			&capacity,
			report_timeout_ms,
			dockwarden_hid_report_callback,
			&context);
	}
	if (status == kIOReturnSuccess) {
		while (!context.done) {
			CFRunLoopRunResult result = CFRunLoopRunInMode(kCFRunLoopDefaultMode, run_loop_timeout_seconds, true);
			if (result == kCFRunLoopRunTimedOut || result == kCFRunLoopRunStopped || result == kCFRunLoopRunFinished) {
				context.status = kIOReturnTimeout;
				break;
			}
		}
		status = context.status;
	}
	IOHIDDeviceUnscheduleFromRunLoop(device, run_loop, kCFRunLoopDefaultMode);
	if (length != NULL && status == kIOReturnSuccess) {
		*length = context.length;
	}
	return status;
}

static int dockwarden_hid_set_report(void *raw_device, const uint8_t *report, size_t length) {
	return dockwarden_hid_wait_for_report(
		(IOHIDDeviceRef)raw_device,
		kIOHIDReportTypeOutput,
		report,
		NULL,
		(CFIndex)length,
		NULL);
}

static int dockwarden_hid_get_report(void *raw_device, uint8_t *report, CFIndex *length) {
	return dockwarden_hid_wait_for_report(
		(IOHIDDeviceRef)raw_device,
		kIOHIDReportTypeInput,
		NULL,
		report,
		*length,
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

	"github.com/fexxdev/dockwarden/internal/domain"
)

const (
	reportSize = 192
)

type Device struct {
	raw     unsafe.Pointer
	manager unsafe.Pointer
}

func Open(target domain.HIDTarget) (*Device, error) {
	var raw unsafe.Pointer
	var manager unsafe.Pointer
	serial := C.CString(target.Serial)
	defer C.free(unsafe.Pointer(serial))
	var matches C.uint32_t
	status := C.dockwarden_hid_open(C.uint32_t(target.VendorID), C.uint32_t(target.ProductID), C.uint32_t(target.LocationID), serial, &raw, &manager, &matches)
	if status != C.kIOReturnSuccess {
		if matches > 1 {
			return nil, fmt.Errorf("ambiguous Dell HID target %04x:%04x: %d matches", target.VendorID, target.ProductID, uint32(matches))
		}
		if status == C.kIOReturnTimeout {
			return nil, fmt.Errorf("Dell HID transaction timed out after 2 seconds; check the USB-C link and power, then retry (IOKit 0x%x)", uint32(status))
		}
		if status == C.kIOReturnNotPermitted {
			return nil, fmt.Errorf("macOS denied direct HID access; grant the terminal or app HID/Input Monitoring permission and retry (IOKit 0x%x)", uint32(status))
		}
		if status == C.kIOReturnExclusiveAccess {
			return nil, fmt.Errorf("Dell HID target is already in use by another process; close other dock tools and retry (IOKit 0x%x)", uint32(status))
		}
		return nil, fmt.Errorf("cannot open Dell HID interface %04x:%04x at %#08x: IOKit 0x%x", target.VendorID, target.ProductID, target.LocationID, uint32(status))
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
		if status == C.kIOReturnTimeout {
			return fmt.Errorf("Dell HID output report timed out after 2 seconds")
		}
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
		if status == C.kIOReturnTimeout {
			return fmt.Errorf("Dell HID input report timed out after 2 seconds")
		}
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
