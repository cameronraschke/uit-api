package types

type FileUploadConstraints struct {
	ImageConstraints       *ImageUploadConstraints
	VideoConstraints       *VideoUploadConstraints
	MaxUploadFileSizeLimit int
}

type ImageUploadConstraints struct {
	MinFileSize                         int
	MaxFileSize                         int
	MaxFileCount                        int
	AcceptedImageExtensionsAndMimeTypes map[string]string
}

type VideoUploadConstraints struct {
	MinFileSize                         int
	MaxFileSize                         int
	MaxFileCount                        int
	AcceptedVideoExtensionsAndMimeTypes map[string]string
}

type HTMLFormConstraints struct {
	GeneralNote   *GeneralNoteConstraints
	InventoryForm *InventoryUpdateFormConstraints
}

type InventoryUpdateFormConstraints struct {
	MaxJSONBytes                 int
	AcquiredDateIsMandatory      bool
	RetiredDateIsMandatory       bool
	IsFunctionalIsMandatory      bool
	DiskRemovedIsMandatory       bool
	LastHardwareCheckIsMandatory bool
	CheckoutBoolIsMandatory      bool
	CheckoutDateIsMandatory      bool
	ReturnDateIsMandatory        bool
}

type GeneralNoteConstraints struct {
	MaxFormBytes int
}
