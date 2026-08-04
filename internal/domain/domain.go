package domain

type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Platform      string       `json:"platform"`
	Command       string       `json:"command"`
	State         string       `json:"state"`
	Dock          *Dock        `json:"dock,omitempty"`
	Checks        []Check      `json:"checks"`
	Update        *UpdateCheck `json:"update,omitempty"`
	Warnings      []string     `json:"warnings"`
}

type Dock struct {
	Manufacturer      string                `json:"manufacturer"`
	Model             string                `json:"model"`
	VendorID          uint16                `json:"vendor_id"`
	ProductID         uint16                `json:"product_id"`
	Serial            string                `json:"serial,omitempty"`
	DescriptorVersion string                `json:"descriptor_version,omitempty"`
	FirmwareVersion   string                `json:"firmware_version"`
	FirmwareSource    string                `json:"firmware_source,omitempty"`
	Devices           []USBDevice           `json:"devices"`
	Services          []ServiceObservation  `json:"services"`
	Firmware          []FirmwareObservation `json:"firmware"`
}

type USBDevice struct {
	Name              string `json:"name"`
	Vendor            string `json:"vendor,omitempty"`
	Product           string `json:"product,omitempty"`
	Class             string `json:"class,omitempty"`
	Serial            string `json:"serial,omitempty"`
	Location          string `json:"location,omitempty"`
	DescriptorVersion string `json:"descriptor_version,omitempty"`
	VendorID          uint16 `json:"vendor_id"`
	ProductID         uint16 `json:"product_id"`
	Depth             int    `json:"depth"`
	Kind              string `json:"kind"`
}

type ServiceObservation struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Evidence string `json:"evidence,omitempty"`
}

type FirmwareObservation struct {
	Component  string `json:"component"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
}

type Check struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Details string `json:"details,omitempty"`
}

type FirmwareCandidate struct {
	SourceURL        string   `json:"source_url"`
	PackageName      string   `json:"package_name"`
	Version          string   `json:"version"`
	ReleaseDate      string   `json:"release_date"`
	SHA256           string   `json:"sha256"`
	Format           string   `json:"format"`
	SupportedOS      []string `json:"supported_os"`
	CompatibleModels []string `json:"compatible_models"`
}

type UpdateCheck struct {
	State     string             `json:"state"`
	SourceURL string             `json:"source_url,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Candidate *FirmwareCandidate `json:"candidate,omitempty"`
}
