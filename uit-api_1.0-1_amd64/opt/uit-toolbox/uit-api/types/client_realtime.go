package types

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxLiveImageBytes = 512 << 20 // 512 MB
	// If LastHeardTimeout is too low, then the job queue
	// gets messed up because the clients temporarily
	// stop sending last_heard while they get ready for a job
	LastHeardTimeout = 20 * time.Second
)

type JobQueueRealtimeDBRow struct {
	ClientUUID           *uuid.UUID
	Tagnumber            *int64
	SerialNumber         *string
	LastHeard            *time.Time
	LastHeardUpdatedInDB *bool
	SystemUptime         *time.Duration
	AppUptime            *time.Duration
	LiveImageBytes       []byte
}

func (dto *JobQueueRealtimeDBRow) ToDTO() (*JobQueueRealtimeDTO, error) {
	if dto == nil {
		return nil, fmt.Errorf("%w: %s", InvalidStructureError, "JobQueueRealtimeDBRow is nil")
	}

	if dto.ClientUUID == nil || *dto.ClientUUID == uuid.Nil {
		return nil, fmt.Errorf("%w: %s", InvalidFieldError, "ClientUUID is nil or empty")
	}

	if err := IsTagnumberInt64PtrValid(dto.Tagnumber); err != nil {
		return nil, err
	}

	if err := IsSystemSerialValid(dereferenceStringPtr(dto.SerialNumber)); err != nil {
		return nil, err
	}

	return &JobQueueRealtimeDTO{
		ClientUUID:           dereferenceUUIDPtr(dto.ClientUUID),
		Tagnumber:            dereferenceInt64Ptr(dto.Tagnumber),
		SerialNumber:         dereferenceStringPtr(dto.SerialNumber),
		LastHeard:            dereferenceTimePtr(dto.LastHeard),
		LastHeardUpdatedInDB: dereferenceBoolPtr(dto.LastHeardUpdatedInDB),
		SystemUptime:         time.Duration(dereferenceDurationPtr(dto.SystemUptime).Seconds()) * time.Second,
		AppUptime:            time.Duration(dereferenceDurationPtr(dto.AppUptime).Seconds()) * time.Second,
		LiveImageBytes:       dto.LiveImageBytes,
	}, nil
}

type JobQueueRealtimeDTO struct {
	ClientUUID           uuid.UUID
	Tagnumber            int64
	SerialNumber         string
	LastHeard            time.Time
	LastHeardUpdatedInDB bool
	SystemUptime         time.Duration
	AppUptime            time.Duration
	LiveImageBytes       []byte
}

type JobQueueDBRow struct {
	ClientUUID             *uuid.UUID     `json:"client_uuid"`
	Tagnumber              *int64         `json:"tagnumber"`
	SystemSerial           *string        `json:"system_serial"`
	SystemManufacturer     *string        `json:"system_manufacturer"`
	SystemModel            *string        `json:"system_model"`
	Location               *string        `json:"location"`
	Department             *string        `json:"department_name"`
	ClientStatus           *string        `json:"client_status"`
	IsBroken               *bool          `json:"is_broken"`
	DiskRemoved            *bool          `json:"disk_removed"`
	TempWarning            *bool          `json:"temp_warning"`
	CheckoutBool           *bool          `json:"checkout_bool"`
	KernelUpdated          *bool          `json:"kernel_updated"`
	LastHeard              *time.Time     `json:"last_heard"`
	SystemUptime           *time.Duration `json:"system_uptime"`     // seconds
	AppUptime              *time.Duration `json:"client_app_uptime"` // seconds
	Online                 *bool          `json:"online"`
	JobActive              *bool          `json:"job_active"`
	JobQueued              *bool          `json:"job_queued"`
	JobQueuedAt            *time.Time     `json:"job_queued_at"`
	QueuePosition          *int64         `json:"job_queue_position"`
	JobName                *string        `json:"job_name"`
	JobNameReadable        *string        `json:"job_name_readable"`
	JobCloneMode           *string        `json:"job_clone_mode"`
	JobEraseMode           *string        `json:"job_erase_mode"`
	JobStatus              *string        `json:"job_status"`
	LastJobTime            *time.Time     `json:"last_job_time"`
	OSInstalled            *bool          `json:"os_installed"`
	OSName                 *string        `json:"os_name"`
	LatestImageInstalled   *bool          `json:"latest_image_installed"`
	DomainJoined           *bool          `json:"domain_joined"`
	DomainName             *string        `json:"ad_domain"`
	DomainNameFormatted    *string        `json:"ad_domain_formatted"`
	BIOSUpdated            *bool          `json:"bios_updated"`
	BIOSVersion            *string        `json:"bios_version"`
	CPUUsage               *float64       `json:"cpu_current_usage"`
	CPUMHz                 *float64       `json:"cpu_mhz"`
	CPUTemp                *float64       `json:"cpu_temp"`
	CPUTempWarning         *bool          `json:"cpu_temp_warning"`
	MemoryUsageKB          *int64         `json:"memory_usage_kb"`
	MemoryCapacityKB       *int64         `json:"memory_capacity_kb"`
	DiskUsage              *float64       `json:"disk_usage"`
	DiskTemp               *float64       `json:"disk_temp"`
	DiskType               *string        `json:"disk_type"`
	DiskSize               *float64       `json:"disk_size_kb"`
	MaxDiskTemp            *float64       `json:"max_disk_temp"`
	DiskTempWarning        *bool          `json:"disk_temp_warning"`
	NetworkLinkStatus      *string        `json:"network_link_status"`
	NetworkLinkSpeed       *float64       `json:"network_link_speed"`
	NetworkUsage           *float64       `json:"network_usage"`
	BatteryCharge          *int64         `json:"battery_charge_pcnt"`
	BatteryStatus          *string        `json:"battery_status"`
	BatteryHealthDeviation *float64       `json:"battery_health_deviation"`
	BatteryHealthPcnt      *float64       `json:"battery_health_pcnt"`
	PluggedIn              *bool          `json:"plugged_in"`
	PowerUsage             *float64       `json:"power_usage"`
}

func (dto *JobQueueDBRow) ToDTO() (*JobQueueDTO, error) {
	if dto == nil {
		return nil, fmt.Errorf("%w: %s", InvalidStructureError, "JobQueueDBRow is nil")
	}

	if dto.ClientUUID == nil || *dto.ClientUUID == uuid.Nil {
		return nil, fmt.Errorf("%w: %s", InvalidFieldError, "ClientUUID is nil or empty")
	}

	if err := IsTagnumberInt64PtrValid(dto.Tagnumber); err != nil {
		return nil, err
	}
	if err := IsSystemSerialValid(dereferenceStringPtr(dto.SystemSerial)); err != nil {
		return nil, err
	}

	return &JobQueueDTO{
		ClientUUID:             dereferenceUUIDPtr(dto.ClientUUID),
		Tagnumber:              dereferenceInt64Ptr(dto.Tagnumber),
		SystemSerial:           dereferenceStringPtr(dto.SystemSerial),
		SystemManufacturer:     dereferenceStringPtr(dto.SystemManufacturer),
		SystemModel:            dereferenceStringPtr(dto.SystemModel),
		Location:               dereferenceStringPtr(dto.Location),
		Department:             dereferenceStringPtr(dto.Department),
		ClientStatus:           dereferenceStringPtr(dto.ClientStatus),
		IsBroken:               dto.IsBroken,
		DiskRemoved:            dto.DiskRemoved,
		TempWarning:            dto.TempWarning,
		CheckoutBool:           dto.CheckoutBool,
		KernelUpdated:          dto.KernelUpdated,
		LastHeard:              dereferenceTimePtr(dto.LastHeard),
		SystemUptime:           time.Duration(dereferenceDurationPtr(dto.SystemUptime).Seconds()) * time.Second,
		AppUptime:              time.Duration(dereferenceDurationPtr(dto.AppUptime).Seconds()) * time.Second,
		Online:                 dto.Online,
		JobActive:              dto.JobActive,
		JobQueued:              dto.JobQueued,
		JobQueuedAt:            dereferenceTimePtr(dto.JobQueuedAt),
		QueuePosition:          dereferenceInt64Ptr(dto.QueuePosition),
		JobName:                dereferenceStringPtr(dto.JobName),
		JobNameReadable:        dereferenceStringPtr(dto.JobNameReadable),
		JobCloneMode:           dereferenceStringPtr(dto.JobCloneMode),
		JobEraseMode:           dereferenceStringPtr(dto.JobEraseMode),
		JobStatus:              dereferenceStringPtr(dto.JobStatus),
		LastJobTime:            dereferenceTimePtr(dto.LastJobTime),
		OSInstalled:            dto.OSInstalled,
		OSName:                 dereferenceStringPtr(dto.OSName),
		LatestImageInstalled:   dto.LatestImageInstalled,
		DomainJoined:           dto.DomainJoined,
		DomainName:             dereferenceStringPtr(dto.DomainName),
		DomainNameFormatted:    dereferenceStringPtr(dto.DomainNameFormatted),
		BIOSUpdated:            dto.BIOSUpdated,
		BIOSVersion:            dereferenceStringPtr(dto.BIOSVersion),
		CPUUsage:               dereferenceFloat64Ptr(dto.CPUUsage),
		CPUMHz:                 dereferenceFloat64Ptr(dto.CPUMHz),
		CPUTemp:                dereferenceFloat64Ptr(dto.CPUTemp),
		CPUTempWarning:         dto.CPUTempWarning,
		MemoryUsageKB:          dereferenceInt64Ptr(dto.MemoryUsageKB),
		MemoryCapacityKB:       dereferenceInt64Ptr(dto.MemoryCapacityKB),
		DiskUsage:              dereferenceFloat64Ptr(dto.DiskUsage),
		DiskTemp:               dereferenceFloat64Ptr(dto.DiskTemp),
		DiskType:               dereferenceStringPtr(dto.DiskType),
		DiskSize:               dereferenceFloat64Ptr(dto.DiskSize),
		MaxDiskTemp:            dereferenceFloat64Ptr(dto.MaxDiskTemp),
		DiskTempWarning:        dto.DiskTempWarning,
		NetworkLinkStatus:      dereferenceStringPtr(dto.NetworkLinkStatus),
		NetworkLinkSpeed:       dereferenceFloat64Ptr(dto.NetworkLinkSpeed),
		NetworkUsage:           dereferenceFloat64Ptr(dto.NetworkUsage),
		BatteryCharge:          dereferenceInt64Ptr(dto.BatteryCharge),
		BatteryStatus:          dereferenceStringPtr(dto.BatteryStatus),
		BatteryHealthDeviation: dereferenceFloat64Ptr(dto.BatteryHealthDeviation),
		BatteryHealthPcnt:      dereferenceFloat64Ptr(dto.BatteryHealthPcnt),
		PluggedIn:              dto.PluggedIn,
		PowerUsage:             dereferenceFloat64Ptr(dto.PowerUsage),
	}, nil
}

type JobQueueDTO struct {
	ClientUUID             uuid.UUID     `json:"client_uuid"`
	Tagnumber              int64         `json:"tagnumber"`
	SystemSerial           string        `json:"system_serial"`
	SystemManufacturer     string        `json:"system_manufacturer"`
	SystemModel            string        `json:"system_model"`
	Location               string        `json:"location"`
	Department             string        `json:"department_name"`
	ClientStatus           string        `json:"client_status"`
	IsBroken               *bool         `json:"is_broken"`
	DiskRemoved            *bool         `json:"disk_removed"`
	TempWarning            *bool         `json:"temp_warning"`
	CheckoutBool           *bool         `json:"checkout_bool"`
	KernelUpdated          *bool         `json:"kernel_updated"`
	LastHeard              time.Time     `json:"last_heard"`
	SystemUptime           time.Duration `json:"system_uptime"`     // seconds
	AppUptime              time.Duration `json:"client_app_uptime"` // seconds
	Online                 *bool         `json:"online"`
	JobActive              *bool         `json:"job_active"`
	JobQueued              *bool         `json:"job_queued"`
	JobQueuedAt            time.Time     `json:"job_queued_at"`
	QueuePosition          int64         `json:"job_queue_position"`
	JobName                string        `json:"job_name"`
	JobNameReadable        string        `json:"job_name_readable"`
	JobCloneMode           string        `json:"job_clone_mode"`
	JobEraseMode           string        `json:"job_erase_mode"`
	JobStatus              string        `json:"job_status"`
	LastJobTime            time.Time     `json:"last_job_time"`
	OSInstalled            *bool         `json:"os_installed"`
	OSName                 string        `json:"os_name"`
	LatestImageInstalled   *bool         `json:"latest_image_installed"`
	DomainJoined           *bool         `json:"domain_joined"`
	DomainName             string        `json:"ad_domain"`
	DomainNameFormatted    string        `json:"ad_domain_formatted"`
	BIOSUpdated            *bool         `json:"bios_updated"`
	BIOSVersion            string        `json:"bios_version"`
	CPUUsage               float64       `json:"cpu_current_usage"`
	CPUMHz                 float64       `json:"cpu_mhz"`
	CPUTemp                float64       `json:"cpu_temp"`
	CPUTempWarning         *bool         `json:"cpu_temp_warning"`
	MemoryUsageKB          int64         `json:"memory_usage_kb"`
	MemoryCapacityKB       int64         `json:"memory_capacity_kb"`
	DiskUsage              float64       `json:"disk_usage"`
	DiskTemp               float64       `json:"disk_temp"`
	DiskType               string        `json:"disk_type"`
	DiskSize               float64       `json:"disk_size_kb"`
	MaxDiskTemp            float64       `json:"max_disk_temp"`
	DiskTempWarning        *bool         `json:"disk_temp_warning"`
	NetworkLinkStatus      string        `json:"network_link_status"`
	NetworkLinkSpeed       float64       `json:"network_link_speed"`
	NetworkUsage           float64       `json:"network_usage"`
	BatteryCharge          int64         `json:"battery_charge_pcnt"`
	BatteryStatus          string        `json:"battery_status"`
	BatteryHealthDeviation float64       `json:"battery_health_deviation"`
	BatteryHealthPcnt      float64       `json:"battery_health_pcnt"`
	PluggedIn              *bool         `json:"plugged_in"`
	PowerUsage             float64       `json:"power_usage"`
}
