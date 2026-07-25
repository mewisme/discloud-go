package selfmanaged

import (
	"time"
)

const (
	ProtectionObfuscated = "obfuscated"
	ProtectionPassword   = "password"
	MetadataVersion      = 1
)

// BundleMeta is the decrypted/deobfuscated JSON payload.
type BundleMeta struct {
	BundleID       string     `json:"bundleId"`
	FormatVersion  int        `json:"formatVersion"`
	CreatedAt      time.Time  `json:"createdAt"`
	Protection     string     `json:"protection"`
	ConfigIdentity string     `json:"configIdentity"`
	GuildID        string     `json:"guildId"`
	ChannelID      string     `json:"channelId"`
	PremiumTier    int        `json:"premiumTier"`
	ChunkSize      int64      `json:"chunkSize"`
	FileCount      int        `json:"fileCount"`
	TotalSize      int64      `json:"totalSize"`
	TotalChunks    int        `json:"totalChunks"`
	Files          []FileMeta `json:"files"`
}

// FileMeta is one logical file inside a bundle.
type FileMeta struct {
	ID           string      `json:"id"`
	LogicalPath  string      `json:"logicalPath"`
	OriginalName string      `json:"originalName"`
	Size         int64       `json:"size"`
	MIMEType     string      `json:"mimeType,omitempty"`
	ChunkCount   int         `json:"chunkCount"`
	SHA256       string      `json:"sha256"`
	UploadedAt   time.Time   `json:"uploadedAt"`
	Chunks       []ChunkMeta `json:"chunks"`
}

// ChunkMeta is one Discord-hosted chunk.
type ChunkMeta struct {
	Index          int    `json:"index"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
	MessageID      string `json:"messageId"`
	AttachmentID   string `json:"attachmentId,omitempty"`
	AttachmentName string `json:"attachmentName,omitempty"`
	ChannelID      string `json:"channelId"`
	BotID          int    `json:"botId"`
}

// RecalcTotals updates aggregate fields from Files.
func (b *BundleMeta) RecalcTotals() {
	b.FileCount = len(b.Files)
	var size int64
	var chunks int
	for _, f := range b.Files {
		size += f.Size
		chunks += len(f.Chunks)
	}
	b.TotalSize = size
	b.TotalChunks = chunks
}
