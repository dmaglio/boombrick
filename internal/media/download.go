package media

// DownloadState is how far a YouTube Music song has got towards being saved
// on the SD card (docs/ui.md, "Download state").
type DownloadState uint8

const (
	Available DownloadState = iota // not downloaded
	Queued
	Downloading
	Saved
	Failed
)
