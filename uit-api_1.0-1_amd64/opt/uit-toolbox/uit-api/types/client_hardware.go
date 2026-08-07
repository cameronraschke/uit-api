package types

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ClientHardwareView struct {
	TransactionUUID              string  `json:"transaction_uuid"`
	Tagnumber                    int64   `json:"tagnumber"`
	SystemSerial                 string  `json:"system_serial"`
	SystemUUID                   string  `json:"system_uuid"`
	SystemManufacturer           string  `json:"system_manufacturer"`
	SystemModel                  string  `json:"system_model"`
	SystemSKU                    string  `json:"system_sku"`
	ProductFamily                string  `json:"product_family,omitempty"`
	ProductName                  string  `json:"product_name,omitempty"`
	DeviceType                   string  `json:"device_type"`
	ChassisType                  string  `json:"chassis_type"`
	MotherboardSerial            string  `json:"motherboard_serial"`
	MotherboardManufacturer      string  `json:"motherboard_manufacturer"`
	CPUManufacturer              string  `json:"cpu_manufacturer"`
	CPUModel                     string  `json:"cpu_model"`
	CPUMaxSpeedMhz               int64   `json:"cpu_max_speed_mhz"`
	CPUCoreCount                 int64   `json:"cpu_core_count"`
	CPUThreadCount               int64   `json:"cpu_thread_count"`
	EthernetMAC                  string  `json:"ethernet_mac"`
	WiFiMAC                      string  `json:"wifi_mac"`
	TPMVersion                   string  `json:"tpm_version"`
	DiskModel                    string  `json:"disk_model"`
	DiskType                     string  `json:"disk_type"`
	DiskSize                     int64   `json:"disk_size_kb"`
	DiskSerial                   string  `json:"disk_serial"`
	DiskWritesKB                 int64   `json:"disk_writes_kb"`
	DiskReadsKB                  int64   `json:"disk_reads_kb"`
	DiskPowerOnHours             int64   `json:"disk_power_on_hours"`
	DiskErrors                   int64   `json:"disk_errors"`
	DiskPowerCycles              int64   `json:"disk_power_cycles"`
	DiskFirmware                 string  `json:"disk_firmware"`
	BatteryModel                 string  `json:"battery_model"`
	BatterySerial                string  `json:"battery_serial"`
	BatteryChargeCycles          int64   `json:"battery_charge_cycles"`
	BatteryCurrentMaxCapacity    float64 `json:"battery_current_max_capacity"`
	BatteryDesignCapacity        float64 `json:"battery_design_capacity"`
	BatteryManufacturer          string  `json:"battery_manufacturer"`
	BatteryManufactureDate       string  `json:"battery_manufacture_date"`
	BatteryManufactureDateParsed time.Time
	BiosVersion                  string    `json:"bios_version"`
	BiosReleaseDate              time.Time `json:"bios_release_date"`
	BiosFirmware                 string    `json:"bios_firmware"`
	MemorySerial                 []string  `json:"memory_serial"`
	MemoryCapacityKB             int64     `json:"memory_capacity_kb"`
	MemorySpeedMHz               int64     `json:"memory_speed_mhz"`
}

type ClientHealthCheck struct {
	TransactionUUID   string    `json:"transaction_uuid"`
	Tagnumber         int64     `json:"tagnumber"`
	SystemSerial      string    `json:"health_system_serial"`
	BIOSVersion       string    `json:"bios_version"`
	BIOSReleaseDate   time.Time `json:"bios_release_date"`
	TPMVersion        string    `json:"health_tpm_version"`
	LastHardwareCheck time.Time `json:"last_hardware_check"`
}

type MemoryDataUpdateRequest struct {
	Tagnumber       int64  `json:"tagnumber"`
	TotalUsageKB    int64  `json:"memory_usage_kb"`
	TotalCapacityKB int64  `json:"memory_capacity_kb"`
	Type            string `json:"type"`
	SpeedMHz        int64  `json:"speed_mhz"`
}

func (m MemoryDataUpdateRequest) IsValid() (errs []error) {
	if err := IsTagnumberInt64Valid(m.Tagnumber); err != nil {
		return append(errs, fmt.Errorf("%w for '%s': %v", InvalidFieldError, "tagnumber", err))
	}

	// Validate other fields in their respective functions
	// if m.TotalUsageKB <= 0 {
	// 	return append(errs, fmt.Errorf("%w: memory usage must be greater than 0", InvalidFieldError))
	// }
	// if m.TotalCapacityKB <= 0 {
	// 	return append(errs, fmt.Errorf("%w: memory capacity must be greater than 0", InvalidFieldError))
	// }
	// if m.SpeedMHz <= 0 {
	// 	return append(errs, fmt.Errorf("%w: memory speed must be greater than 0", InvalidFieldError))
	// }

	return errs
}

type CPUDataUpdateRequest struct {
	Tagnumber     int64   `json:"tagnumber"`
	UsagePercent  float64 `json:"cpu_current_usage"`
	MHz           float64 `json:"cpu_current_mhz"`
	MillidegreesC float64 `json:"cpu_millidegrees_c"`
}

func (c CPUDataUpdateRequest) IsValid() error {
	if err := IsTagnumberInt64Valid(c.Tagnumber); err != nil {
		return fmt.Errorf("%w for '%s': %v", InvalidFieldError, "tagnumber", err)
	}
	if c.UsagePercent < 0 || c.UsagePercent > 110 {
		return fmt.Errorf("%w: CPU usage percent must be between 0 and 100", InvalidFieldError)
	}
	if c.MHz <= 0 {
		return fmt.Errorf("%w: CPU MHz must be greater than 0", InvalidFieldError)
	}
	if c.MillidegreesC <= 0 {
		return fmt.Errorf("%w: CPU temperature must be greater than 0", InvalidFieldError)
	}
	return nil
}

type NetworkData struct {
	Tagnumber    int64  `json:"tagnumber"`
	NetworkUsage *int64 `json:"network_usage"`
	LinkSpeed    *int64 `json:"link_speed"`
}

type BatteryDataRequest struct {
	TransactionUUID           uuid.UUID `json:"transaction_uuid"`
	UpdatedFromWindows        bool      `json:"updated_from_windows"`
	TimeStamp                 time.Time `json:"timestamp"`
	Tagnumber                 int64     `json:"tagnumber"`
	SystemSerial              string    `json:"system_serial"`
	BatteryChargeCycles       int64     `json:"battery_charge_cycles"`
	BatteryChargePcnt         float64   `json:"battery_charge_pcnt"`
	BatteryCurrentMaxCapacity float64   `json:"battery_current_max_capacity"`
	BatteryDesignCapacity     float64   `json:"battery_design_capacity"`
	BatteryManufactureDate    string    `json:"battery_manufacture_date"`
	BatteryManufacturer       string    `json:"battery_manufacturer"`
	BatteryModel              string    `json:"battery_model"`
	BatterySerial             string    `json:"battery_serial"`
}

func (b *BatteryDataRequest) IsValid() (errs []error) {
	if b == nil {
		return append(errs, fmt.Errorf("battery data request is nil"))
	}

	// transactionUUID is optional, generate a new one if not provided
	if b.TransactionUUID == uuid.Nil {
		newUUID, err := uuid.NewV7()
		if err != nil {
			return append(errs, fmt.Errorf("%w: failed to generate transaction UUID", InvalidFieldError))
		}
		b.TransactionUUID = newUUID
	}

	// timestamp is optional, use current UTC time if not provided
	if b.TimeStamp.IsZero() {
		b.TimeStamp = time.Now().UTC()
	} else {
		b.TimeStamp = b.TimeStamp.UTC()
	}

	// tagnumber
	if err := IsTagnumberInt64Valid(b.Tagnumber); err != nil {
		return append(errs, fmt.Errorf("%w for '%s': %v", InvalidFieldError, "tagnumber", err))
	}

	// systemSerial is optional, but if provided, it must not be empty
	if b.SystemSerial != "" {
		if err := IsSystemSerialValid(b.SystemSerial); err != nil {
			return append(errs, fmt.Errorf("%w for '%s': %v", InvalidFieldError, "system_serial", err))
		}
	}

	if b.BatteryChargePcnt < 0 || b.BatteryChargePcnt > 100 {
		return append(errs, fmt.Errorf("%w: battery charge percent must be between 0 and 100", InvalidFieldError))
	}

	return errs
}
